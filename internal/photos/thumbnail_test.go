package photos

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateThumbnail(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "test_src.png")
	destPath := filepath.Join(tempDir, "test_thumb.jpg")

	// 创建一个 300x150 彩色图片
	img := image.NewRGBA(image.Rect(0, 0, 300, 150))
	for y := 0; y < 150; y++ {
		for x := 0; x < 300; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 128, A: 255})
		}
	}
	f, err := os.Create(srcPath)
	if err != nil {
		t.Fatalf("create src png: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("encode src png: %v", err)
	}
	f.Close()

	if err := GenerateThumbnail(srcPath, destPath, 120); err != nil {
		t.Fatalf("GenerateThumbnail failed: %v", err)
	}

	thumbFile, err := os.Open(destPath)
	if err != nil {
		t.Fatalf("open thumb: %v", err)
	}
	defer thumbFile.Close()

	thumbImg, err := jpeg.Decode(thumbFile)
	if err != nil {
		t.Fatalf("decode thumb: %v", err)
	}

	bounds := thumbImg.Bounds()
	if bounds.Dx() != 120 || bounds.Dy() != 120 {
		t.Fatalf("expected 120x120, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestGeneratePreviewPreservesAspectRatioAndDoesNotUpscale(t *testing.T) {
	tempDir := t.TempDir()
	widePath := filepath.Join(tempDir, "wide.png")
	widePreviewPath := filepath.Join(tempDir, "wide-preview.jpg")
	smallPath := filepath.Join(tempDir, "small.png")
	smallPreviewPath := filepath.Join(tempDir, "small-preview.jpg")

	writePNG := func(path string, width, height int) {
		t.Helper()
		file, err := os.Create(path)
		if err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
		defer file.Close()
		if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
			t.Fatalf("encode %s: %v", path, err)
		}
	}
	decodeBounds := func(path string) image.Rectangle {
		t.Helper()
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		defer file.Close()
		decoded, err := jpeg.Decode(file)
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return decoded.Bounds()
	}

	writePNG(widePath, 300, 150)
	if err := GeneratePreview(widePath, widePreviewPath, 120); err != nil {
		t.Fatalf("GeneratePreview wide image failed: %v", err)
	}
	if bounds := decodeBounds(widePreviewPath); bounds.Dx() != 120 || bounds.Dy() != 60 {
		t.Fatalf("expected 120x60 aspect preview, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	writePNG(smallPath, 60, 30)
	if err := GeneratePreview(smallPath, smallPreviewPath, 120); err != nil {
		t.Fatalf("GeneratePreview small image failed: %v", err)
	}
	if bounds := decodeBounds(smallPreviewPath); bounds.Dx() != 60 || bounds.Dy() != 30 {
		t.Fatalf("expected small image to remain 60x30, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}
