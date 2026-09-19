package photos

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
)

// GenerateThumbnail 将源图像裁剪居中正方形并缩放到 size*size，以 JPEG 格式保存到 destPath
func GenerateThumbnail(srcPath, destPath string, size int) error {
	if size <= 0 {
		size = 120
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open src image: %w", err)
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode src image: %w", err)
	}

	thumb := createSquareThumbnail(src, size)

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("mkdir cache dir: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create dest thumbnail: %w", err)
	}
	defer out.Close()

	// 采用标准 JPEG 编码，质量 82 (体积小、清晰度高)
	if err := jpeg.Encode(out, thumb, &jpeg.Options{Quality: 82}); err != nil {
		return fmt.Errorf("encode thumbnail: %w", err)
	}

	return nil
}

func createSquareThumbnail(src image.Image, size int) *image.RGBA {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, size, size))
	}

	// 1. 确定居中裁剪的正方形区域
	side := w
	if h < side {
		side = h
	}
	startX := bounds.Min.X + (w-side)/2
	startY := bounds.Min.Y + (h-side)/2

	dst := image.NewRGBA(image.Rect(0, 0, size, size))

	// 2. 双线性插值缩放至 size x size
	scale := float64(side) / float64(size)
	for dy := 0; dy < size; dy++ {
		sy := float64(startY) + (float64(dy)+0.5)*scale - 0.5
		if sy < float64(bounds.Min.Y) {
			sy = float64(bounds.Min.Y)
		}
		if sy > float64(bounds.Max.Y-1) {
			sy = float64(bounds.Max.Y - 1)
		}
		y0 := int(sy)
		y1 := y0 + 1
		if y1 >= bounds.Max.Y {
			y1 = bounds.Max.Y - 1
		}
		fy := sy - float64(y0)

		for dx := 0; dx < size; dx++ {
			sx := float64(startX) + (float64(dx)+0.5)*scale - 0.5
			if sx < float64(bounds.Min.X) {
				sx = float64(bounds.Min.X)
			}
			if sx > float64(bounds.Max.X-1) {
				sx = float64(bounds.Max.X - 1)
			}
			x0 := int(sx)
			x1 := x0 + 1
			if x1 >= bounds.Max.X {
				x1 = bounds.Max.X - 1
			}
			fx := sx - float64(x0)

			c00 := src.At(x0, y0)
			c10 := src.At(x1, y0)
			c01 := src.At(x0, y1)
			c11 := src.At(x1, y1)

			r00, g00, b00, a00 := c00.RGBA()
			r10, g10, b10, a10 := c10.RGBA()
			r01, g01, b01, a01 := c01.RGBA()
			r11, g11, b11, a11 := c11.RGBA()

			rTop := float64(r00)*(1-fx) + float64(r10)*fx
			gTop := float64(g00)*(1-fx) + float64(g10)*fx
			bTop := float64(b00)*(1-fx) + float64(b10)*fx
			aTop := float64(a00)*(1-fx) + float64(a10)*fx

			rBottom := float64(r01)*(1-fx) + float64(r11)*fx
			gBottom := float64(g01)*(1-fx) + float64(g11)*fx
			bBottom := float64(b01)*(1-fx) + float64(b11)*fx
			aBottom := float64(a01)*(1-fx) + float64(a11)*fx

			r := uint8((rTop*(1-fy) + rBottom*fy) / 257)
			g := uint8((gTop*(1-fy) + gBottom*fy) / 257)
			b := uint8((bTop*(1-fy) + bBottom*fy) / 257)
			a := uint8((aTop*(1-fy) + aBottom*fy) / 257)

			dst.SetRGBA(dx, dy, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}

	return dst
}

// FallbackThumbnailBytes 生成带相机图标风格的轻量通用占位图
func FallbackThumbnailBytes() []byte {
	// 简单的 1x1 灰色像素 fallback
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 200, G: 210, B: 220, A: 255})
		}
	}
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}
