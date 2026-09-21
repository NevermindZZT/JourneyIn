package photos

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	migration := `
CREATE TABLE IF NOT EXISTS photo_index (
  id TEXT PRIMARY KEY,
  file_path TEXT UNIQUE NOT NULL,
  file_name TEXT NOT NULL,
  file_size INTEGER NOT NULL,
  mod_time INTEGER NOT NULL,
  taken_at TEXT NOT NULL,
  has_gps INTEGER NOT NULL DEFAULT 0,
  lat_wgs84 REAL,
  lng_wgs84 REAL,
  lat_gcj02 REAL,
  lng_gcj02 REAL,
  lat_bd09ll REAL,
  lng_bd09ll REAL,
  camera_make TEXT,
  camera_model TEXT,
  lens_model TEXT,
  exposure_time_s REAL,
  f_number REAL,
  iso INTEGER,
  focal_length_mm REAL,
  exposure_bias_ev REAL,
  metadata_version INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_photo_index_has_gps ON photo_index(has_gps);
CREATE INDEX IF NOT EXISTS idx_photo_index_taken_at ON photo_index(taken_at);
`
	if _, err := db.Exec(migration); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return db
}

func createTestJpegWithGPS(t *testing.T, filePath string, latDeg, lngDeg float64, takenAt time.Time) {
	var tiff bytes.Buffer
	tiff.Write([]byte{'I', 'I', 0x2A, 0x00})
	binary.Write(&tiff, binary.LittleEndian, uint32(8))
	binary.Write(&tiff, binary.LittleEndian, uint16(2))

	dateTimeOffset := uint32(8 + 2 + 2*12 + 4 + 2 + 4*12 + 4)
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0132))
	binary.Write(&tiff, binary.LittleEndian, uint16(2))
	binary.Write(&tiff, binary.LittleEndian, uint32(20))
	binary.Write(&tiff, binary.LittleEndian, dateTimeOffset)

	gpsIfdOffset := uint32(8 + 2 + 2*12 + 4)
	binary.Write(&tiff, binary.LittleEndian, uint16(0x8825))
	binary.Write(&tiff, binary.LittleEndian, uint16(4))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, gpsIfdOffset)

	binary.Write(&tiff, binary.LittleEndian, uint32(0))

	binary.Write(&tiff, binary.LittleEndian, uint16(4))
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0001))
	binary.Write(&tiff, binary.LittleEndian, uint16(2))
	binary.Write(&tiff, binary.LittleEndian, uint32(2))
	tiff.Write([]byte{'N', 0, 0, 0})

	latDataOffset := dateTimeOffset + 20
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0002))
	binary.Write(&tiff, binary.LittleEndian, uint16(5))
	binary.Write(&tiff, binary.LittleEndian, uint32(3))
	binary.Write(&tiff, binary.LittleEndian, latDataOffset)

	binary.Write(&tiff, binary.LittleEndian, uint16(0x0003))
	binary.Write(&tiff, binary.LittleEndian, uint16(2))
	binary.Write(&tiff, binary.LittleEndian, uint32(2))
	tiff.Write([]byte{'E', 0, 0, 0})

	lngDataOffset := latDataOffset + 24
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0004))
	binary.Write(&tiff, binary.LittleEndian, uint16(5))
	binary.Write(&tiff, binary.LittleEndian, uint32(3))
	binary.Write(&tiff, binary.LittleEndian, lngDataOffset)

	binary.Write(&tiff, binary.LittleEndian, uint32(0))

	timeBytes := append([]byte(takenAt.Format("2006:01:02 15:04:05")), 0)
	tiff.Write(timeBytes)

	binary.Write(&tiff, binary.LittleEndian, uint32(latDeg))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, uint32(0))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, uint32(0))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))

	binary.Write(&tiff, binary.LittleEndian, uint32(lngDeg))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, uint32(0))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, uint32(0))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))

	var jpegBuf bytes.Buffer
	jpegBuf.Write([]byte{0xFF, 0xD8})
	jpegBuf.Write([]byte{0xFF, 0xE1})

	exifPrefix := []byte{'E', 'x', 'i', 'f', 0, 0}
	app1Payload := append(exifPrefix, tiff.Bytes()...)
	app1Len := uint16(len(app1Payload) + 2)
	binary.Write(&jpegBuf, binary.BigEndian, app1Len)
	jpegBuf.Write(app1Payload)

	// 写入简单的有效图像数据
	img := image.NewRGBA(image.Rect(0, 0, 60, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 60; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var rawImg bytes.Buffer
	_ = jpeg.Encode(&rawImg, img, &jpeg.Options{Quality: 70})
	jpegBuf.Write(rawImg.Bytes()[2:]) // 跳过前面的 SOI

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, jpegBuf.Bytes(), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestPhotoServiceLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tempDir := t.TempDir()
	photosDir := filepath.Join(tempDir, "photos")
	cacheDir := filepath.Join(tempDir, "cache")

	// 1. 创建子文件夹和测试照片
	subDir := filepath.Join(photosDir, "2024", "trip1")
	photo1Path := filepath.Join(subDir, "p1.jpg")
	t1 := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	createTestJpegWithGPS(t, photo1Path, 39, 116, t1)

	svc := NewService(db, photosDir, cacheDir, nil)
	if !svc.IsEnabled() {
		t.Fatalf("expected service to be enabled")
	}

	ctx := context.Background()
	// 2. 执行同步扫描
	if err := svc.ScanSync(ctx); err != nil {
		t.Fatalf("ScanSync failed: %v", err)
	}

	// 3. 检查状态
	status, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.TotalPhotos != 1 || status.GPSPhotos != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
	photoBeforeBackfill, err := svc.GetPhotoByID(ctx, generatePhotoID(photosDir, photo1Path))
	if err != nil || photoBeforeBackfill.MetadataVersion != photoMetadataVersion {
		t.Fatalf("unexpected initial metadata version: %+v, err=%v", photoBeforeBackfill, err)
	}
	// An old index row is re-parsed even if its image bytes are unchanged.
	if _, err := db.Exec("UPDATE photo_index SET metadata_version = 0 WHERE id = ?", photoBeforeBackfill.ID); err != nil {
		t.Fatalf("mark photo metadata stale: %v", err)
	}
	if err := svc.ScanSync(ctx); err != nil {
		t.Fatalf("backfill stale capture metadata: %v", err)
	}
	photoAfterBackfill, err := svc.GetPhotoByID(ctx, photoBeforeBackfill.ID)
	if err != nil || photoAfterBackfill.MetadataVersion != photoMetadataVersion {
		t.Fatalf("metadata backfill failed: %+v, err=%v", photoAfterBackfill, err)
	}

	// 4. 获取 Atlas 照片 (GCJ02 与 BD09LL)
	atlasItemsGCJ, err := svc.GetAtlasPhotos(ctx, "gcj02")
	if err != nil || len(atlasItemsGCJ) != 1 {
		t.Fatalf("expected 1 atlas item (gcj02), got %d, err=%v", len(atlasItemsGCJ), err)
	}
	if math.Abs(atlasItemsGCJ[0].Lat-39) > 0.5 {
		t.Fatalf("unexpected lat: %f", atlasItemsGCJ[0].Lat)
	}
	if atlasItemsGCJ[0].ThumbURL == "" || atlasItemsGCJ[0].PreviewURL == "" || atlasItemsGCJ[0].CacheVersion == 0 {
		t.Fatalf("atlas rendition metadata missing: %+v", atlasItemsGCJ[0])
	}

	atlasItemsBD, err := svc.GetAtlasPhotos(ctx, "bd09ll")
	if err != nil || len(atlasItemsBD) != 1 {
		t.Fatalf("expected 1 atlas item (bd09ll), got %d, err=%v", len(atlasItemsBD), err)
	}
	pID := atlasItemsGCJ[0].ID

	// 5. 缩略图生成与缓存读取
	thumbBytes, err := svc.GetThumbnail(ctx, pID, 100)
	if err != nil || len(thumbBytes) == 0 {
		t.Fatalf("GetThumbnail failed: %v", err)
	}
	// 再次获取，命中缓存
	thumbCached, err := svc.GetThumbnail(ctx, pID, 100)
	if err != nil || len(thumbCached) != len(thumbBytes) {
		t.Fatalf("cached thumbnail mismatch: %v", err)
	}

	// 6. Lightbox preview uses a separate, versioned, aspect-ratio-preserving cache.
	previewBytes, err := svc.GetPreview(ctx, pID, PreviewSmallEdge)
	if err != nil || len(previewBytes) == 0 {
		t.Fatalf("GetPreview failed: %v", err)
	}
	photo, err := svc.GetPhotoByID(ctx, pID)
	if err != nil {
		t.Fatalf("GetPhotoByID failed: %v", err)
	}
	previewPath := svc.renditionCachePath("previews", pID, photo.ModTime, PreviewSmallEdge)
	if _, err := os.Stat(previewPath); err != nil {
		t.Fatalf("expected preview cache file at %s: %v", previewPath, err)
	}
	previewCached, err := svc.GetPreview(ctx, pID, PreviewSmallEdge)
	if err != nil || len(previewCached) != len(previewBytes) {
		t.Fatalf("cached preview mismatch: %v", err)
	}
	if _, err := svc.GetPreview(ctx, pID, 1200); err == nil {
		t.Fatal("expected unsupported preview size to fail")
	}

	// A file update keeps its stable ID but invalidates the prior versioned cache.
	updatedModTime := time.Unix(photo.ModTime+2, 0)
	if err := os.Chtimes(photo1Path, updatedModTime, updatedModTime); err != nil {
		t.Fatalf("update photo mod time: %v", err)
	}
	if err := svc.ScanSync(ctx); err != nil {
		t.Fatalf("scan changed photo: %v", err)
	}
	if _, err := os.Stat(previewPath); !os.IsNotExist(err) {
		t.Fatalf("expected old preview rendition to be removed, stat err=%v", err)
	}
	updatedPhoto, err := svc.GetPhotoByID(ctx, pID)
	if err != nil || updatedPhoto.ModTime != updatedModTime.Unix() {
		t.Fatalf("unexpected updated photo metadata: %+v, err=%v", updatedPhoto, err)
	}
	if _, err := svc.GetPreview(ctx, pID, PreviewSmallEdge); err != nil {
		t.Fatalf("generate preview after update: %v", err)
	}
	updatedPreviewPath := svc.renditionCachePath("previews", pID, updatedPhoto.ModTime, PreviewSmallEdge)
	if _, err := os.Stat(updatedPreviewPath); err != nil {
		t.Fatalf("expected updated preview cache file at %s: %v", updatedPreviewPath, err)
	}

	// 7. 安全读取文件路径
	safePath, err := svc.GetPhotoFilePath(ctx, pID)
	if err != nil || safePath != photo1Path {
		t.Fatalf("unexpected file path: %s, err=%v", safePath, err)
	}

	// 7. 测试删除文件后的同步清理
	os.Remove(photo1Path)
	if err := svc.ScanSync(ctx); err != nil {
		t.Fatalf("second scan failed: %v", err)
	}
	statusAfterDel, _ := svc.Status(ctx)
	if statusAfterDel.TotalPhotos != 0 || statusAfterDel.GPSPhotos != 0 {
		t.Fatalf("expected 0 photos after deletion, got %+v", statusAfterDel)
	}
}
