package uploads

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrImageTooLarge       = errors.New("image is too large")
	ErrUnsupportedImage    = errors.New("unsupported image type")
	ErrInvalidImage        = errors.New("invalid image")
	ErrImageDimensions     = errors.New("image dimensions exceed limits")
	generatedImageFilename = regexp.MustCompile(`^[a-f0-9]{32}\.(?:jpg|png)$`)
)

type ImageConfig struct {
	Directory string
	MaxBytes  int64
	MaxWidth  int
	MaxHeight int
	MaxPixels int64
	Scanner   Scanner
}

type ImageStore struct {
	directory string
	maxBytes  int64
	maxWidth  int
	maxHeight int
	maxPixels int64
	scanner   Scanner
}

func NewImageStore(config ImageConfig) (*ImageStore, error) {
	if strings.TrimSpace(config.Directory) == "" {
		return nil, errors.New("uploads: image directory is required")
	}
	if config.MaxBytes < 1024 || config.MaxWidth < 1 || config.MaxHeight < 1 || config.MaxPixels < 1 {
		return nil, errors.New("uploads: image limits must be positive")
	}
	if config.Scanner == nil {
		return nil, errors.New("uploads: malware scanner is required")
	}
	directory, err := filepath.Abs(config.Directory)
	if err != nil {
		return nil, fmt.Errorf("resolve image directory: %w", err)
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("create image directory: %w", err)
	}
	return &ImageStore{directory: directory, maxBytes: config.MaxBytes, maxWidth: config.MaxWidth, maxHeight: config.MaxHeight, maxPixels: config.MaxPixels, scanner: config.Scanner}, nil
}

func (store *ImageStore) Save(ctx context.Context, originalName string, source io.Reader, declaredSize int64) (string, error) {
	extension := strings.ToLower(filepath.Ext(originalName))
	expectedFormat, expectedMIME, canonicalExtension := imageType(extension)
	if expectedFormat == "" {
		return "", ErrUnsupportedImage
	}
	if declaredSize > store.maxBytes {
		return "", ErrImageTooLarge
	}

	data, err := io.ReadAll(io.LimitReader(source, store.maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	if int64(len(data)) > store.maxBytes {
		return "", ErrImageTooLarge
	}
	if err := store.scanner.Scan(ctx, data); err != nil {
		return "", err
	}
	if len(data) == 0 || http.DetectContentType(data[:min(len(data), 512)]) != expectedMIME {
		return "", ErrInvalidImage
	}

	imageConfig, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != expectedFormat || imageConfig.Width < 1 || imageConfig.Height < 1 {
		return "", ErrInvalidImage
	}
	pixels := int64(imageConfig.Width) * int64(imageConfig.Height)
	if imageConfig.Width > store.maxWidth || imageConfig.Height > store.maxHeight || pixels > store.maxPixels {
		return "", ErrImageDimensions
	}
	decodedImage, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || format != expectedFormat {
		return "", ErrInvalidImage
	}

	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); err != nil {
		return "", fmt.Errorf("generate image name: %w", err)
	}
	filename := hex.EncodeToString(identifier) + canonicalExtension
	temporary, err := os.CreateTemp(store.directory, ".image-*")
	if err != nil {
		return "", fmt.Errorf("create temporary image: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", fmt.Errorf("secure temporary image: %w", err)
	}
	limitedOutput := &limitedImageWriter{writer: temporary, remaining: store.maxBytes}
	if expectedFormat == "jpeg" {
		err = jpeg.Encode(limitedOutput, decodedImage, &jpeg.Options{Quality: 88})
	} else {
		err = (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(limitedOutput, decodedImage)
	}
	if err != nil {
		if errors.Is(err, ErrImageTooLarge) {
			return "", ErrImageTooLarge
		}
		return "", fmt.Errorf("encode sanitized image: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync sanitized image: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close sanitized image: %w", err)
	}
	if err := os.Rename(temporaryName, filepath.Join(store.directory, filename)); err != nil {
		return "", fmt.Errorf("commit sanitized image: %w", err)
	}
	committed = true
	return filename, nil
}

func (store *ImageStore) Delete(filename string) error {
	if !store.ValidFilename(filename) {
		return nil
	}
	if err := os.Remove(filepath.Join(store.directory, filename)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete image: %w", err)
	}
	return nil
}

func (store *ImageStore) Open(filename string) (*os.File, error) {
	if !store.ValidFilename(filename) {
		return nil, os.ErrNotExist
	}
	return os.Open(filepath.Join(store.directory, filename))
}

func (store *ImageStore) ValidFilename(filename string) bool {
	return generatedImageFilename.MatchString(filename)
}

func (store *ImageStore) MaxBytes() int64 {
	return store.maxBytes
}

func ContentType(filename string) string {
	return mime.TypeByExtension(filepath.Ext(filename))
}

func imageType(extension string) (format, contentType, canonicalExtension string) {
	switch extension {
	case ".jpg", ".jpeg":
		return "jpeg", "image/jpeg", ".jpg"
	case ".png":
		return "png", "image/png", ".png"
	default:
		return "", "", ""
	}
}

type limitedImageWriter struct {
	writer    io.Writer
	remaining int64
}

func (writer *limitedImageWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > writer.remaining {
		return 0, ErrImageTooLarge
	}
	written, err := writer.writer.Write(data)
	writer.remaining -= int64(written)
	return written, err
}
