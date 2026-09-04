package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/uploads"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	multipartFormOverhead = int64(1 << 20)
	maxProductImages      = 8
)

var errTooManyProductImages = errors.New("product image limit reached")

func (h *EmployeeHandler) UpdateProductImage(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	database := h.database.WithContext(c.Request.Context())
	var product models.Product
	if err := database.First(&product, uri.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
		} else {
			c.String(http.StatusInternalServerError, "Could not load product")
		}
		return
	}

	var imageCount int64
	if err := database.Model(&models.ProductImage{}).Where("product_id = ?", product.ID).Count(&imageCount).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load product images")
		return
	}
	if imageCount >= maxProductImages {
		h.respondToImageError(c, errTooManyProductImages)
		return
	}

	h.limitImageUpload(c)
	newFilenames, err := h.saveProductImages(c, true, maxProductImages-int(imageCount))
	if err != nil {
		h.respondToImageError(c, err)
		return
	}
	err = database.Transaction(func(transaction *gorm.DB) error {
		var lockedProduct models.Product
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProduct, product.ID).Error; err != nil {
			return err
		}
		var currentCount int64
		if err := transaction.Model(&models.ProductImage{}).Where("product_id = ?", product.ID).Count(&currentCount).Error; err != nil {
			return err
		}
		if currentCount+int64(len(newFilenames)) > maxProductImages {
			return errTooManyProductImages
		}
		var lastPosition struct{ Position int64 }
		if err := transaction.Model(&models.ProductImage{}).Select("COALESCE(MAX(position), -1) AS position").Where("product_id = ?", product.ID).Scan(&lastPosition).Error; err != nil {
			return err
		}
		images := make([]models.ProductImage, 0, len(newFilenames))
		for index, filename := range newFilenames {
			images = append(images, models.ProductImage{ProductID: product.ID, Filename: filename, Position: uint(lastPosition.Position + int64(index) + 1)})
		}
		if err := transaction.Create(&images).Error; err != nil {
			return err
		}
		if lockedProduct.ImageFilename == "" {
			return transaction.Model(&lockedProduct).Update("image_filename", newFilenames[0]).Error
		}
		return nil
	})
	if err != nil {
		h.cleanupProductImages(newFilenames)
		if errors.Is(err, errTooManyProductImages) {
			h.respondToImageError(c, err)
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		slog.ErrorContext(c.Request.Context(), "product image records could not be created", "product_id", product.ID, "error", err)
		c.String(http.StatusInternalServerError, "Could not update product images")
		return
	}
	slog.InfoContext(c.Request.Context(), "product images added", "product_id", product.ID, "image_count", len(newFilenames))
	c.Redirect(http.StatusSeeOther, "/employee/products?status=images-added")
}

func (h *EmployeeHandler) limitImageUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.imageStore.MaxBytes()*maxProductImages+multipartFormOverhead)
}

func (h *EmployeeHandler) saveProductImages(c *gin.Context, required bool, maximum int) ([]string, error) {
	form, err := c.MultipartForm()
	if err != nil {
		if !required && errors.Is(err, http.ErrMissingFile) {
			return nil, nil
		}
		if !errors.Is(err, http.ErrMissingFile) && !isUploadTooLarge(err) {
			return nil, fmt.Errorf("%w: malformed multipart form", uploads.ErrInvalidImage)
		}
		return nil, err
	}
	defer form.RemoveAll()
	headers := form.File["images"]
	if len(headers) == 0 {
		headers = form.File["image"]
	}
	if len(headers) == 0 {
		if required {
			return nil, http.ErrMissingFile
		}
		return nil, nil
	}
	if maximum < 1 || len(headers) > maximum {
		return nil, errTooManyProductImages
	}

	filenames := make([]string, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			h.cleanupProductImages(filenames)
			return nil, err
		}
		filename, saveErr := h.imageStore.Save(c.Request.Context(), header.Filename, file, header.Size)
		closeErr := file.Close()
		if saveErr != nil {
			h.cleanupProductImages(filenames)
			return nil, saveErr
		}
		if closeErr != nil {
			h.cleanupProductImage(filename)
			h.cleanupProductImages(filenames)
			return nil, closeErr
		}
		filenames = append(filenames, filename)
	}
	return filenames, nil
}

func (h *EmployeeHandler) SetProductCoverImage(c *gin.Context) {
	var uri validation.ProductImageURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	database := h.database.WithContext(c.Request.Context())
	err := database.Transaction(func(transaction *gorm.DB) error {
		var product models.Product
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, uri.ProductID).Error; err != nil {
			return err
		}
		var image models.ProductImage
		if err := transaction.Where("id = ? AND product_id = ?", uri.ImageID, product.ID).First(&image).Error; err != nil {
			return err
		}
		return transaction.Model(&product).Update("image_filename", image.Filename).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
		} else {
			slog.ErrorContext(c.Request.Context(), "product cover image could not be updated", "product_id", uri.ProductID, "image_id", uri.ImageID, "error", err)
			c.String(http.StatusInternalServerError, "Could not update cover image")
		}
		return
	}
	slog.InfoContext(c.Request.Context(), "product cover image updated", "product_id", uri.ProductID, "image_id", uri.ImageID)
	c.Redirect(http.StatusSeeOther, "/employee/products?status=cover-updated")
}

func (h *EmployeeHandler) DeleteProductImage(c *gin.Context) {
	var uri validation.ProductImageURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	database := h.database.WithContext(c.Request.Context())
	var deletedFilename string
	err := database.Transaction(func(transaction *gorm.DB) error {
		var product models.Product
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, uri.ProductID).Error; err != nil {
			return err
		}
		var image models.ProductImage
		if err := transaction.Where("id = ? AND product_id = ?", uri.ImageID, product.ID).First(&image).Error; err != nil {
			return err
		}
		deletedFilename = image.Filename
		if err := transaction.Delete(&image).Error; err != nil {
			return err
		}
		if product.ImageFilename != image.Filename {
			return nil
		}
		var replacement models.ProductImage
		err := transaction.Where("product_id = ?", product.ID).Order("position ASC, id ASC").First(&replacement).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return transaction.Model(&product).Update("image_filename", replacement.Filename).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
		} else {
			slog.ErrorContext(c.Request.Context(), "product image could not be deleted", "product_id", uri.ProductID, "image_id", uri.ImageID, "error", err)
			c.String(http.StatusInternalServerError, "Could not delete product image")
		}
		return
	}
	h.cleanupProductImage(deletedFilename)
	slog.InfoContext(c.Request.Context(), "product image deleted", "product_id", uri.ProductID, "image_id", uri.ImageID)
	c.Redirect(http.StatusSeeOther, "/employee/products?status=image-deleted")
}

func (h *EmployeeHandler) respondToImageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errTooManyProductImages):
		c.String(http.StatusConflict, "A product can have up to %d images", maxProductImages)
	case isUploadTooLarge(err), errors.Is(err, uploads.ErrImageTooLarge):
		c.String(http.StatusRequestEntityTooLarge, "Product image is too large")
	case errors.Is(err, uploads.ErrScannerUnavailable):
		slog.Error("product image security scan unavailable", "error", err)
		c.String(http.StatusServiceUnavailable, "Image security scan is temporarily unavailable")
	case errors.Is(err, uploads.ErrThreatDetected):
		slog.Warn("unsafe product image rejected", "reason", "malware_detected")
		c.String(http.StatusBadRequest, "Product image failed the security check")
	case errors.Is(err, uploads.ErrUnsupportedImage), errors.Is(err, uploads.ErrInvalidImage), errors.Is(err, uploads.ErrImageDimensions), errors.Is(err, http.ErrMissingFile):
		c.String(http.StatusBadRequest, "Product image must be a valid JPEG or PNG within the allowed dimensions")
	default:
		slog.Error("product image processing failed", "error", err)
		c.String(http.StatusInternalServerError, "Could not process product image")
	}
}

func (h *EmployeeHandler) cleanupProductImages(filenames []string) {
	for _, filename := range filenames {
		h.cleanupProductImage(filename)
	}
}

func (h *EmployeeHandler) cleanupProductImage(filename string) {
	if filename == "" {
		return
	}
	if err := h.imageStore.Delete(filename); err != nil {
		slog.Warn("product image cleanup failed", "error", err)
	}
}

func isUploadTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError) || errors.Is(err, multipart.ErrMessageTooLarge) || strings.Contains(strings.ToLower(err.Error()), "request body too large")
}
