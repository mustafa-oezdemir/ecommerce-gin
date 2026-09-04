package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/uploads"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"gorm.io/gorm"
)

const (
	maxProductImageUploadBytes = int64(15 << 20)
	maxProductImageFiles       = 4
)

func (h *EmployeeHandler) UploadProductImages(c *gin.Context) {
	var uri validation.ProductIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var product models.Product
	if err := db.DB.WithContext(c.Request.Context()).First(&product, uri.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
		} else {
			c.String(http.StatusInternalServerError, "Could not load product")
		}
		return
	}

	h.limitImageUpload(c)
	newFilenames, err := h.saveProductImages(c, true)
	if err != nil {
		h.respondToImageError(c, err)
		return
	}
	savedFilenames := newFilenames
	if err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		start := int64(0)
		if err := tx.Model(&models.ProductImage{}).Where("product_id = ?", product.ID).Count(&start).Error; err != nil {
			return err
		}
		if product.ImageFilename == "" {
			if err := tx.Model(&models.Product{}).Where("id = ? AND (image_filename IS NULL OR image_filename = '')", product.ID).Update("image_filename", newFilenames[0]).Error; err != nil {
				return err
			}
			newFilenames = newFilenames[1:]
		}
		if len(newFilenames) == 0 {
			return nil
		}
		return tx.Create(productImageRecords(product.ID, start, newFilenames)).Error
	}); err != nil {
		h.cleanupProductImages(savedFilenames)
		c.String(http.StatusInternalServerError, "Could not update product images")
		return
	}
	c.Redirect(http.StatusFound, "/employee/products")
}

func (h *EmployeeHandler) limitImageUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxProductImageUploadBytes)
}

func (h *EmployeeHandler) saveProductImages(c *gin.Context, required bool) ([]string, error) {
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
	if len(headers) > maxProductImageFiles {
		return nil, fmt.Errorf("%w: at most %d files are allowed", uploads.ErrInvalidImage, maxProductImageFiles)
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

func (h *EmployeeHandler) respondToImageError(c *gin.Context, err error) {
	switch {
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

func (h *EmployeeHandler) cleanupProductImage(filename string) {
	if filename == "" {
		return
	}
	if err := h.imageStore.Delete(filename); err != nil {
		slog.Warn("product image cleanup failed", "error", err)
	}
}

func (h *EmployeeHandler) cleanupProductImages(filenames []string) {
	for _, filename := range filenames {
		h.cleanupProductImage(filename)
	}
}

func productImageRecords(productID uint, start int64, filenames []string) []models.ProductImage {
	records := make([]models.ProductImage, 0, len(filenames))
	for _, filename := range filenames {
		records = append(records, models.ProductImage{ProductID: productID, Filename: filename, Position: start})
		start++
	}
	return records
}

func isUploadTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError) || errors.Is(err, multipart.ErrMessageTooLarge) || strings.Contains(strings.ToLower(err.Error()), "request body too large")
}
