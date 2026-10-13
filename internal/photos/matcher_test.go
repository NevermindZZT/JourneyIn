package photos

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func matchingRecord(id, path string, takenAt time.Time, lat, lng float64, hasGPS bool) photoAtlasRecord {
	item := PhotoAtlasItem{ID: id, FileName: id + ".jpg", TakenAt: takenAt.UTC().Format(time.RFC3339), Lat: lng, Lng: lat, LocationSource: "gps"}
	return photoAtlasRecord{item: item, filePath: path, takenAt: takenAt, takenAtSource: "exif_original", takenAtOffsetKnown: true, hasGPS: hasGPS, latWGS84: lat, lngWGS84: lng, cameraMake: "Example", cameraModel: "Camera X"}
}

func TestNearestStableGPSAnchorUsesConfiguredWindow(t *testing.T) {
	targetTime := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	target := matchingRecord("target", "", targetTime, 30, 120, false)
	before := matchingRecord("before", "", targetTime.Add(-20*time.Minute), 30, 120, true)
	after := matchingRecord("after", "", targetTime.Add(15*time.Minute), 30.0001, 120.0001, true)
	anchor, delta, ok := nearestStableGPSAnchor(target, []photoAtlasRecord{before, after}, 30*time.Minute)
	if !ok || anchor.item.ID != "after" || delta != 15*time.Minute {
		t.Fatalf("expected nearest after anchor, got id=%s delta=%v ok=%v", anchor.item.ID, delta, ok)
	}
	if _, _, ok := nearestStableGPSAnchor(target, []photoAtlasRecord{before, after}, 14*time.Minute); ok {
		t.Fatal("expected candidates outside configured window to be rejected")
	}
	if _, _, ok := nearestStableGPSAnchor(target, []photoAtlasRecord{before, after}, 15*time.Minute); !ok {
		t.Fatal("expected exact window boundary to be included")
	}
}

func TestNearestStableGPSAnchorRejectsMovementAndBreaksTies(t *testing.T) {
	targetTime := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	target := matchingRecord("target", "", targetTime, 30, 120, false)
	before := matchingRecord("before", "", targetTime.Add(-time.Minute), 30, 120, true)
	after := matchingRecord("after", "", targetTime.Add(time.Minute), 30.0001, 120.0001, true)
	anchor, _, ok := nearestStableGPSAnchor(target, []photoAtlasRecord{before, after}, 15*time.Minute)
	if !ok || anchor.item.ID != "before" {
		t.Fatalf("equal-time tie must deterministically prefer the earlier anchor, got %+v", anchor)
	}
	farAfter := matchingRecord("far", "", targetTime.Add(2*time.Minute), 31, 121, true)
	if _, _, ok := nearestStableGPSAnchor(target, []photoAtlasRecord{before, farAfter}, 15*time.Minute); ok {
		t.Fatal("expected conflicting before/after GPS points to be rejected")
	}
}

func TestPhotoMatchGroupRequiresSameFolderCameraAndTimeBasis(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	taken := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	base := matchingRecord("a", filepath.Join(root, "trip", "a.jpg"), taken, 30, 120, true)
	key := photoMatchGroupKey(root, base)
	if key == "" {
		t.Fatal("expected group key for complete capture metadata")
	}
	same := matchingRecord("b", filepath.Join(root, "trip", "b.jpg"), taken.Add(time.Minute), 30, 120, false)
	if photoMatchGroupKey(root, same) != key {
		t.Fatal("same folder/camera/time basis should share a group")
	}
	otherFolder := matchingRecord("c", filepath.Join(root, "other", "c.jpg"), taken, 30, 120, true)
	if photoMatchGroupKey(root, otherFolder) == key {
		t.Fatal("different folders must not match")
	}
	otherCamera := base
	otherCamera.cameraModel = "Camera Y"
	if photoMatchGroupKey(root, otherCamera) == key {
		t.Fatal("different camera models must not match")
	}
	unknownZone := base
	unknownZone.takenAtOffsetKnown = false
	if photoMatchGroupKey(root, unknownZone) == key {
		t.Fatal("known and unknown timezone bases must not mix")
	}
	unknownTime := base
	unknownTime.takenAtSource = "file_mod_time"
	if photoMatchGroupKey(root, unknownTime) != "" {
		t.Fatal("filesystem modification time must never be used for inference")
	}
	noCamera := base
	noCamera.cameraModel = ""
	if photoMatchGroupKey(root, noCamera) != "" {
		t.Fatal("missing camera identity must not be inferred")
	}
}

func TestGetAtlasPhotosWithMatchingIsOptInAndDoesNotChain(t *testing.T) {
	db := setupTestDB(t)
	root := filepath.Clean(t.TempDir())
	service := NewService(db, root, "", nil)
	baseTime := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	insertIndexedPhotoForMatch(t, db, root, "gps", "album", baseTime, true, 30, 120, "exif_original")
	insertIndexedPhotoForMatch(t, db, root, "missing", "album", baseTime.Add(20*time.Minute), false, 0, 0, "exif_original")
	insertIndexedPhotoForMatch(t, db, root, "missing-later", "album", baseTime.Add(40*time.Minute), false, 0, 0, "exif_original")
	insertIndexedPhotoForMatch(t, db, root, "mtime", "album", baseTime.Add(10*time.Minute), false, 0, 0, "file_mod_time")
	insertIndexedPhotoForMatch(t, db, root, "other-folder", "elsewhere", baseTime.Add(time.Minute), false, 0, 0, "exif_original")
	exact, err := service.GetAtlasPhotos(context.Background(), "gcj02")
	if err != nil {
		t.Fatal(err)
	}
	if len(exact) != 1 || exact[0].ID != "gps" || exact[0].LocationSource != "gps" {
		t.Fatalf("default GPS-only behavior changed: %+v", exact)
	}
	matched, err := service.GetAtlasPhotosWithMatching(context.Background(), "gcj02", true, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected one exact and one inferred photo, got %+v", matched)
	}
	var estimate *PhotoAtlasItem
	for index := range matched {
		if matched[index].LocationSource == "time_match" {
			estimate = &matched[index]
		}
	}
	if estimate == nil || estimate.ID != "missing" || estimate.MatchedPhotoID != "gps" || estimate.MatchDeltaSeconds != 1200 || math.Abs(estimate.Lat-30) > 1e-9 || math.Abs(estimate.Lng-120) > 1e-9 {
		t.Fatalf("unexpected inferred photo: %+v", estimate)
	}
}

func insertIndexedPhotoForMatch(t *testing.T, db *sql.DB, root, id, folder string, takenAt time.Time, hasGPS bool, lat, lng float64, source string) {
	t.Helper()
	path := filepath.Join(root, folder, id+".jpg")
	gps := 0
	var latValue, lngValue any
	if hasGPS {
		gps = 1
		latValue = lat
		lngValue = lng
	}
	_, err := db.Exec(`INSERT INTO photo_index(id,file_path,file_name,file_size,mod_time,taken_at,taken_at_source,taken_at_offset_known,has_gps,lat_wgs84,lng_wgs84,lat_gcj02,lng_gcj02,lat_bd09ll,lng_bd09ll,camera_make,camera_model,metadata_version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, path, id+".jpg", 100, takenAt.Unix(), takenAt.UTC().Format(time.RFC3339), source, 1, gps, latValue, lngValue, latValue, lngValue, latValue, lngValue, "Example", "Camera X", photoMetadataVersion, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert photo index row: %v", err)
	}
}
