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

const multipartFormOverhead = int64(1 << 20)

func (h *EmployeeHandler) UpdateProductImage(c *gin.Context) {
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
	newFilename, err := h.saveProductImage(c, true)
	if err != nil {
		h.respondToImageError(c, err)
		return
	}
	oldFilename := product.ImageFilename
	update := db.DB.WithContext(c.Request.Context()).Model(&models.Product{}).Where("id = ?", product.ID)
	if oldFilename == "" {
		update = update.Where("image_filename IS NULL OR image_filename = ''")
	} else {
		update = update.Where("image_filename = ?", oldFilename)
	}
	result := update.Update("image_filename", newFilename)
	if result.Error != nil {
		h.cleanupProductImage(newFilename)
		c.String(http.StatusInternalServerError, "Could not update product image")
		return
	}
	if result.RowsAffected != 1 {
		h.cleanupProductImage(newFilename)
		c.String(http.StatusConflict, "Product image changed; please try again")
		return
	}
	h.cleanupProductImage(oldFilename)
	c.Redirect(http.StatusFound, "/employee/products")
}

func (h *EmployeeHandler) limitImageUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.imageStore.MaxBytes()+multipartFormOverhead)
}

func (h *EmployeeHandler) saveProductImage(c *gin.Context, required bool) (string, error) {
	header, err := c.FormFile("image")
	if err != nil {
		if !required && errors.Is(err, http.ErrMissingFile) {
			return "", nil
		}
		if !errors.Is(err, http.ErrMissingFile) && !isUploadTooLarge(err) {
			return "", fmt.Errorf("%w: malformed multipart form", uploads.ErrInvalidImage)
		}
		return "", err
	}
	file, err := header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()
	return h.imageStore.Save(c.Request.Context(), header.Filename, file, header.Size)
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

func isUploadTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError) || errors.Is(err, multipart.ErrMessageTooLarge) || strings.Contains(strings.ToLower(err.Error()), "request body too large")
}
