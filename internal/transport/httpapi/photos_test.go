package httpapi

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	journeyin "journeyin"
	"journeyin/internal/application"
	journeymaps "journeyin/internal/maps"
	"journeyin/internal/photos"
	journeyshare "journeyin/internal/share"
	"journeyin/internal/store"
)

func TestPhotosHTTPEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	photosDir := filepath.Join(tempDir, "user_photos")
	cacheDir := filepath.Join(tempDir, "cache")
	dbPath := filepath.Join(tempDir, "test.db")

	// 准备一张测试图片
	imgFile := filepath.Join(photosDir, "trip", "pic1.jpg")
	if err := os.MkdirAll(filepath.Dir(imgFile), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(imgFile)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: 120, G: 180, B: 240, A: 255})
		}
	}
	_ = jpeg.Encode(f, img, &jpeg.Options{Quality: 80})
	f.Close()

	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), dbPath, migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	photoSvc := photos.NewService(db.DB(), photosDir, cacheDir, nil)
	if err := photoSvc.ScanSync(context.Background()); err != nil {
		t.Fatalf("ScanSync failed: %v", err)
	}

	webFS, _ := fs.Sub(journeyin.WebFS, "web/dist")
	schemaFS, _ := fs.Sub(journeyin.SchemaFS, "schemas")
	provider := &httpPlanningProvider{}
	registry := journeymaps.NewRegistry(provider)
	mapService := application.NewMapService(db, registry, 2, 0)
	app := application.NewTripService(db)
	app.SetMapService(mapService)
	api := NewServer(app, webFS, schemaFS, "test", nil)
	api.SetMapRegistry(registry, "")
	api.SetMapService(mapService)
	api.SetShareService(journeyshare.NewService(journeyshare.NewSQLiteStore(db)), "http://example.test")
	api.SetSyncStore(db)
	api.SetSettingsStore(db)
	api.SetPhotoService(photoSvc)
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	// 1. 测试 GET /api/v1/photos/status
	statusResp, err := http.Get(server.URL + "/api/v1/photos/status")
	if err != nil || statusResp.StatusCode != http.StatusOK {
		t.Fatalf("get status error: %v, code=%d", err, statusResp.StatusCode)
	}
	var status photos.PhotoStatus
	_ = json.NewDecoder(statusResp.Body).Decode(&status)
	statusResp.Body.Close()
	if !status.Enabled || status.TotalPhotos != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}

	// 2. 测试 POST /api/v1/photos/sync
	syncResp, err := http.Post(server.URL+"/api/v1/photos/sync", "application/json", nil)
	if err != nil || syncResp.StatusCode != http.StatusOK {
		t.Fatalf("sync error: %v, code=%d", err, syncResp.StatusCode)
	}
	syncResp.Body.Close()

	// 3. 测试 GET /api/v1/photos/atlas
	atlasResp, err := http.Get(server.URL + "/api/v1/photos/atlas?crs=gcj02")
	if err != nil || atlasResp.StatusCode != http.StatusOK {
		t.Fatalf("atlas photos error: %v, code=%d", err, atlasResp.StatusCode)
	}
	var atlasList []photos.PhotoAtlasItem
	_ = json.NewDecoder(atlasResp.Body).Decode(&atlasList)
	atlasResp.Body.Close()

	// 4. 测试缩略图与原图获取
	// 插入一条带 gps 的临时数据以供测试缩略图和原图
	row := db.DB().QueryRow("SELECT id FROM photo_index LIMIT 1")
	var photoID string
	if err := row.Scan(&photoID); err != nil {
		t.Fatalf("scan photo id error: %v", err)
	}
	thumbResp, err := http.Get(server.URL + "/api/v1/photos/" + photoID + "/thumbnail?size=100")
	if err != nil || thumbResp.StatusCode != http.StatusOK {
		t.Fatalf("thumbnail error: %v, code=%d", err, thumbResp.StatusCode)
	}
	thumbResp.Body.Close()

	fileResp, err := http.Get(server.URL + "/api/v1/photos/" + photoID + "/file")
	if err != nil || fileResp.StatusCode != http.StatusOK {
		t.Fatalf("file error: %v, code=%d", err, fileResp.StatusCode)
	}
	fileResp.Body.Close()
}