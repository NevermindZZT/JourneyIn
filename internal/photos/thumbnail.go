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

// GenerateThumbnail 将源图像裁剪居中正方形并缩放到 size*size，以 JPEG 格式保存到 destPath。
// 缩略图仅用于地图图钉和小尺寸列表，不应用于照片预览。
func GenerateThumbnail(srcPath, destPath string, size int) error {
	if size <= 0 {
		size = 120
	}
	src, err := decodeImage(srcPath)
	if err != nil {
		return err
	}
	return writeJPEG(destPath, createSquareThumbnail(src, size), 82)
}

// GeneratePreview 按原始比例缩放图片，限制长边为 maxEdge，不裁切且不放大原图。
// 预览图用于 Lightbox，避免浏览器在普通浏览时下载与解码原始大图。
func GeneratePreview(srcPath, destPath string, maxEdge int) error {
	if maxEdge <= 0 {
		maxEdge = 1600
	}
	src, err := decodeImage(srcPath)
	if err != nil {
		return err
	}
	return writeJPEG(destPath, createAspectPreview(src, maxEdge), 84)
}

func decodeImage(srcPath string) (image.Image, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open src image: %w", err)
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode src image: %w", err)
	}
	return src, nil
}

func writeJPEG(destPath string, img image.Image, quality int) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("mkdir cache dir: %w", err)
	}
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination image: %w", err)
	}
	defer out.Close()
	if err := jpeg.Encode(out, img, &jpeg.Options{Quality: quality}); err != nil {
		return fmt.Errorf("encode jpeg: %w", err)
	}
	return nil
}

func createSquareThumbnail(src image.Image, size int) *image.RGBA {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, size, size))
	}

	// 确定居中裁剪的正方形区域。
	side := w
	if h < side {
		side = h
	}
	startX := bounds.Min.X + (w-side)/2
	startY := bounds.Min.Y + (h-side)/2
	return resizeImage(src, image.Rect(startX, startY, startX+side, startY+side), size, size)
}

func createAspectPreview(src image.Image, maxEdge int) *image.RGBA {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}

	previewW, previewH := w, h
	if w > maxEdge || h > maxEdge {
		if w >= h {
			previewW = maxEdge
			previewH = max(1, (h*maxEdge+w/2)/w)
		} else {
			previewH = maxEdge
			previewW = max(1, (w*maxEdge+h/2)/h)
		}
	}
	return resizeImage(src, bounds, previewW, previewH)
}

// resizeImage 对 source 区域做双线性插值缩放。
func resizeImage(src image.Image, source image.Rectangle, width, height int) *image.RGBA {
	if width <= 0 || height <= 0 || source.Dx() <= 0 || source.Dy() <= 0 {
		return image.NewRGBA(image.Rect(0, 0, max(1, width), max(1, height)))
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	scaleX := float64(source.Dx()) / float64(width)
	scaleY := float64(source.Dy()) / float64(height)
	maxX := source.Max.X - 1
	maxY := source.Max.Y - 1

	for dy := 0; dy < height; dy++ {
		sy := float64(source.Min.Y) + (float64(dy)+0.5)*scaleY - 0.5
		if sy < float64(source.Min.Y) {
			sy = float64(source.Min.Y)
		}
		if sy > float64(maxY) {
			sy = float64(maxY)
		}
		y0 := int(sy)
		y1 := min(y0+1, maxY)
		fy := sy - float64(y0)

		for dx := 0; dx < width; dx++ {
			sx := float64(source.Min.X) + (float64(dx)+0.5)*scaleX - 0.5
			if sx < float64(source.Min.X) {
				sx = float64(source.Min.X)
			}
			if sx > float64(maxX) {
				sx = float64(maxX)
			}
			x0 := int(sx)
			x1 := min(x0+1, maxX)
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

// FallbackThumbnailBytes 生成轻量通用占位图。
func FallbackThumbnailBytes() []byte {
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
