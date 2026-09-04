package uploads

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
)

type scannerFunc func(context.Context, []byte) error

func (function scannerFunc) Scan(ctx context.Context, data []byte) error {
	return function(ctx, data)
}

func TestImageStoreValidatesAndSanitizesPNG(t *testing.T) {
	directory := t.TempDir()
	scanned := false
	store, err := NewImageStore(ImageConfig{Directory: directory, MaxBytes: 1024 * 1024, MaxWidth: 100, MaxHeight: 100, MaxPixels: 10_000, Scanner: scannerFunc(func(_ context.Context, data []byte) error {
		scanned = len(data) > 0
		return nil
	})})
	if err != nil {
		t.Fatalf("create image store: %v", err)
	}

	payloadMarker := []byte("appended-payload-must-be-removed")
	input := append(testPNG(t, 8, 6), payloadMarker...)
	filename, err := store.Save(context.Background(), "product.PNG", bytes.NewReader(input), int64(len(input)))
	if err != nil {
		t.Fatalf("save image: %v", err)
	}
	if !scanned || !store.ValidFilename(filename) || !strings.HasSuffix(filename, ".png") {
		t.Fatalf("unexpected stored image %q", filename)
	}

	file, err := store.Open(filename)
	if err != nil {
		t.Fatalf("open stored image: %v", err)
	}
	stored, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		t.Fatalf("read stored image: %v", err)
	}
	if bytes.Contains(stored, payloadMarker) {
		t.Fatal("sanitized image retained appended payload")
	}
	if _, _, err := image.Decode(bytes.NewReader(stored)); err != nil {
		t.Fatalf("stored image is not decodable: %v", err)
	}
	if err := store.Delete(filename); err != nil {
		t.Fatalf("delete stored image: %v", err)
	}
	if _, err := os.Stat(store.directory + string(os.PathSeparator) + filename); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected image deletion, got %v", err)
	}
}

func TestImageStoreRejectsUnsafeContent(t *testing.T) {
	cleanScanner := scannerFunc(func(context.Context, []byte) error { return nil })
	store, err := NewImageStore(ImageConfig{Directory: t.TempDir(), MaxBytes: 1024, MaxWidth: 10, MaxHeight: 10, MaxPixels: 100, Scanner: cleanScanner})
	if err != nil {
		t.Fatalf("create image store: %v", err)
	}
	pngData := testPNG(t, 2, 2)

	if _, err := store.Save(context.Background(), "product.exe", bytes.NewReader(pngData), int64(len(pngData))); !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("expected unsupported extension, got %v", err)
	}
	if _, err := store.Save(context.Background(), "product.jpg", bytes.NewReader(pngData), int64(len(pngData))); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("expected MIME mismatch, got %v", err)
	}
	if _, err := store.Save(context.Background(), "product.png", bytes.NewReader(testPNG(t, 11, 1)), 100); !errors.Is(err, ErrImageDimensions) {
		t.Fatalf("expected dimension rejection, got %v", err)
	}
	if _, err := store.Save(context.Background(), "product.png", bytes.NewReader(make([]byte, 1025)), 1025); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestImageStoreRejectsScannerThreat(t *testing.T) {
	store, err := NewImageStore(ImageConfig{Directory: t.TempDir(), MaxBytes: 1024, MaxWidth: 10, MaxHeight: 10, MaxPixels: 100, Scanner: scannerFunc(func(context.Context, []byte) error { return ErrThreatDetected })})
	if err != nil {
		t.Fatalf("create image store: %v", err)
	}
	data := testPNG(t, 2, 2)
	if _, err := store.Save(context.Background(), "product.png", bytes.NewReader(data), int64(len(data))); !errors.Is(err, ErrThreatDetected) {
		t.Fatalf("expected threat rejection, got %v", err)
	}
	entries, err := os.ReadDir(store.directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("threat should not be persisted: entries=%d err=%v", len(entries), err)
	}
}

func TestImageStoreRejectsPathsOutsideRoot(t *testing.T) {
	store, err := NewImageStore(ImageConfig{Directory: t.TempDir(), MaxBytes: 1024, MaxWidth: 10, MaxHeight: 10, MaxPixels: 100, Scanner: scannerFunc(func(context.Context, []byte) error { return nil })})
	if err != nil {
		t.Fatalf("create image store: %v", err)
	}
	for _, filename := range []string{"../secret.png", `..\\secret.png`, "/secret.png", "product.png/secret"} {
		if _, err := store.Open(filename); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected unsafe path %q to be rejected, got %v", filename, err)
		}
	}
}

func TestImageStoreScansBeforeParsingUntrustedContent(t *testing.T) {
	store, err := NewImageStore(ImageConfig{Directory: t.TempDir(), MaxBytes: 1024, MaxWidth: 10, MaxHeight: 10, MaxPixels: 100, Scanner: scannerFunc(func(context.Context, []byte) error { return ErrThreatDetected })})
	if err != nil {
		t.Fatalf("create image store: %v", err)
	}
	payload := []byte("malware-disguised-with-an-image-extension")
	if _, err := store.Save(context.Background(), "product.png", bytes.NewReader(payload), int64(len(payload))); !errors.Is(err, ErrThreatDetected) {
		t.Fatalf("expected security scan before image parsing, got %v", err)
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	picture.Set(0, 0, color.RGBA{R: 20, G: 40, B: 60, A: 255})
	if err := png.Encode(&output, picture); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return output.Bytes()
}
