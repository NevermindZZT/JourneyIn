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
