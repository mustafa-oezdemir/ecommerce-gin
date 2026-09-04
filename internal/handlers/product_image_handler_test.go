package handlers

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/uploads"
)

type cleanImageScanner struct{}

func (cleanImageScanner) Scan(context.Context, []byte) error { return nil }

func TestSaveProductImagesStoresEverySelectedImage(t *testing.T) {
	directory := t.TempDir()
	handler := testEmployeeImageHandler(t, directory)
	context := multipartImageContext(t, []testUpload{{name: "front.png", data: smallPNG(t)}, {name: "back.png", data: smallPNG(t)}})

	filenames, err := handler.saveProductImages(context, true, maxProductImages)
	if err != nil {
		t.Fatalf("save product images: %v", err)
	}
	if len(filenames) != 2 {
		t.Fatalf("got %d images, want 2", len(filenames))
	}
	for _, filename := range filenames {
		if _, err := os.Stat(filepath.Join(directory, filename)); err != nil {
			t.Fatalf("stored image %q is unavailable: %v", filename, err)
		}
	}
}

func TestSaveProductImagesCleansUpWhenAnyImageFails(t *testing.T) {
	directory := t.TempDir()
	handler := testEmployeeImageHandler(t, directory)
	context := multipartImageContext(t, []testUpload{{name: "front.png", data: smallPNG(t)}, {name: "payload.exe", data: smallPNG(t)}})

	if _, err := handler.saveProductImages(context, true, maxProductImages); err == nil {
		t.Fatal("expected unsafe second file to fail")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read image directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed multi-upload left %d stored files", len(entries))
	}
}

func TestSaveProductImagesEnforcesProductLimitBeforeStorage(t *testing.T) {
	directory := t.TempDir()
	handler := testEmployeeImageHandler(t, directory)
	context := multipartImageContext(t, []testUpload{{name: "front.png", data: smallPNG(t)}, {name: "back.png", data: smallPNG(t)}})

	if _, err := handler.saveProductImages(context, true, 1); err != errTooManyProductImages {
		t.Fatalf("got %v, want product image limit error", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read image directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatal("over-limit upload wrote product files")
	}
}

type testUpload struct {
	name string
	data []byte
}

func testEmployeeImageHandler(t *testing.T, directory string) *EmployeeHandler {
	t.Helper()
	store, err := uploads.NewImageStore(uploads.ImageConfig{Directory: directory, MaxBytes: 1024 * 1024, MaxWidth: 100, MaxHeight: 100, MaxPixels: 10_000, Scanner: cleanImageScanner{}})
	if err != nil {
		t.Fatalf("create image store: %v", err)
	}
	return &EmployeeHandler{imageStore: store}
}

func multipartImageContext(t *testing.T, files []testUpload) *gin.Context {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range files {
		part, err := writer.CreateFormFile("images", file.name)
		if err != nil {
			t.Fatalf("create multipart file: %v", err)
		}
		if _, err := part.Write(file.data); err != nil {
			t.Fatalf("write multipart file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart form: %v", err)
	}
	request := httptest.NewRequest("POST", "/employee/products/1/images", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	return context
}

func smallPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 249, G: 115, B: 22, A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, picture); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return output.Bytes()
}
