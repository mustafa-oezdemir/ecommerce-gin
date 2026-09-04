package handlers

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/uploads"
)

type cleanImageScanner struct{}

func (cleanImageScanner) Scan(context.Context, []byte) error { return nil }

func TestSaveProductImagesAcceptsMultipleFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := uploads.NewImageStore(uploads.ImageConfig{
		Directory: t.TempDir(),
		MaxBytes:  1024 * 1024,
		MaxWidth:  100,
		MaxHeight: 100,
		MaxPixels: 10_000,
		Scanner:   cleanImageScanner{},
	})
	if err != nil {
		t.Fatalf("create image store: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, name := range []string{"first.png", "second.png", "third.png", "fourth.png"} {
		part, err := writer.CreateFormFile("images", name)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(testProductImage(t)); err != nil {
			t.Fatalf("write image: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/employee/products/1/image", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	handler := NewEmployeeHandler(store)
	handler.limitImageUpload(context)

	filenames, err := handler.saveProductImages(context, true)
	if err != nil {
		t.Fatalf("save product images: %v", err)
	}
	if len(filenames) != 4 {
		t.Fatalf("saved %d images, want 4", len(filenames))
	}
	for _, filename := range filenames {
		if !store.ValidFilename(filename) {
			t.Errorf("stored invalid generated filename %q", filename)
		}
	}
}

func TestSaveProductImagesRejectsRequestsOver15MB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := uploads.NewImageStore(uploads.ImageConfig{
		Directory: t.TempDir(),
		MaxBytes:  maxProductImageUploadBytes,
		MaxWidth:  100,
		MaxHeight: 100,
		MaxPixels: 10_000,
		Scanner:   cleanImageScanner{},
	})
	if err != nil {
		t.Fatalf("create image store: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("images", "large.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(make([]byte, maxProductImageUploadBytes)); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/employee/products/1/image", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	handler := NewEmployeeHandler(store)
	handler.limitImageUpload(context)

	if _, err := handler.saveProductImages(context, true); !isUploadTooLarge(err) {
		t.Fatalf("expected request too large error, got %v", err)
	}
}

func testProductImage(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 20, G: 40, B: 60, A: 255})
	if err := png.Encode(&output, picture); err != nil {
		t.Fatalf("encode image: %v", err)
	}
	return output.Bytes()
}
