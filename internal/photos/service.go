package photos

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const DefaultPhotoMatchWindowMinutes = 30

func ValidPhotoMatchWindowMinutes(value int) bool {
	return value == 15 || value == 30 || value == 60 || value == 120
}

type PhotoCaptureMetadata struct {
	CameraMake     string   `json:"camera_make,omitempty"`
	CameraModel    string   `json:"camera_model,omitempty"`
	LensModel      string   `json:"lens_model,omitempty"`
	ExposureTimeS  *float64 `json:"exposure_time_s,omitempty"`
	FNumber        *float64 `json:"f_number,omitempty"`
	ISO            *int     `json:"iso,omitempty"`
	FocalLengthMM  *float64 `json:"focal_length_mm,omitempty"`
	ExposureBiasEV *float64 `json:"exposure_bias_ev,omitempty"`
}

func (m PhotoCaptureMetadata) HasValues() bool {
	return m.CameraMake != "" || m.CameraModel != "" || m.LensModel != "" || m.ExposureTimeS != nil || m.FNumber != nil || m.ISO != nil || m.FocalLengthMM != nil || m.ExposureBiasEV != nil
}

type PhotoRecord struct {
	ID                 string               `json:"id"`
	FilePath           string               `json:"file_path"`
	FileName           string               `json:"file_name"`
	FileSize           int64                `json:"file_size"`
	ModTime            int64                `json:"mod_time"`
	TakenAt            time.Time            `json:"taken_at"`
	TakenAtSource      string               `json:"taken_at_source"`
	TakenAtOffsetKnown bool                 `json:"taken_at_offset_known"`
	HasGPS             bool                 `json:"has_gps"`
	LatWGS84           float64              `json:"lat_wgs84,omitempty"`
	LngWGS84           float64              `json:"lng_wgs84,omitempty"`
	LatGCJ02           float64              `json:"lat_gcj02,omitempty"`
	LngGCJ02           float64              `json:"lng_gcj02,omitempty"`
	LatBD09LL          float64              `json:"lat_bd09ll,omitempty"`
	LngBD09LL          float64              `json:"lng_bd09ll,omitempty"`
	Capture            PhotoCaptureMetadata `json:"capture"`
	MetadataVersion    int                  `json:"metadata_version"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

type PhotoAtlasItem struct {
	ID                string                `json:"id"`
	FileName          string                `json:"file_name"`
	TakenAt           string                `json:"taken_at"`
	Lat               float64               `json:"lat"`
	Lng               float64               `json:"lng"`
	ThumbURL          string                `json:"thumb_url"`
	PreviewURL        string                `json:"preview_url"`
	CacheVersion      int64                 `json:"cache_version"`
	LocationSource    string                `json:"location_source"`
	MatchedPhotoID    string                `json:"matched_photo_id,omitempty"`
	MatchDeltaSeconds int64                 `json:"match_delta_seconds,omitempty"`
	Capture           *PhotoCaptureMetadata `json:"capture,omitempty"`
}

type PhotoStatus struct {
	Enabled     bool   `json:"enabled"`
	RootDir     string `json:"root_dir"`
	TotalPhotos int    `json:"total_photos"`
	GPSPhotos   int    `json:"gps_photos"`
	Scanning    bool   `json:"scanning"`
	LastScanAt  string `json:"last_scan_at,omitempty"`
}

const photoMetadataVersion = 3

type Service struct {
	db         *sql.DB
	rootDir    string
	cacheDir   string
	logger     *slog.Logger
	mu         sync.RWMutex
	scanning   bool
	lastScanAt time.Time
}

func NewService(db *sql.DB, rootDir, cacheDir string, logger *slog.Logger) *Service {
	cleanRoot := ""
	if strings.TrimSpace(rootDir) != "" {
		if abs, err := filepath.Abs(rootDir); err == nil {
			cleanRoot = abs
		} else {
			cleanRoot = filepath.Clean(rootDir)
		}
	}
	cleanCache := ""
	if strings.TrimSpace(cacheDir) != "" {
		if abs, err := filepath.Abs(cacheDir); err == nil {
			cleanCache = abs
		} else {
			cleanCache = filepath.Clean(cacheDir)
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		db:       db,
		rootDir:  cleanRoot,
		cacheDir: cleanCache,
		logger:   logger,
	}
}

func (s *Service) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rootDir != ""
}

func (s *Service) RootDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rootDir
}

func (s *Service) SetRootDir(newRoot string) {
	cleanRoot := ""
	if strings.TrimSpace(newRoot) != "" {
		if abs, err := filepath.Abs(newRoot); err == nil {
			cleanRoot = abs
		} else {
			cleanRoot = filepath.Clean(newRoot)
		}
	}
	s.mu.Lock()
	s.rootDir = cleanRoot
	s.mu.Unlock()
}

func (s *Service) Status(ctx context.Context) (PhotoStatus, error) {
	s.mu.RLock()
	scanning := s.scanning
	lastScan := s.lastScanAt
	s.mu.RUnlock()

	status := PhotoStatus{
		Enabled:  s.IsEnabled(),
		RootDir:  s.rootDir,
		Scanning: scanning,
	}
	if !lastScan.IsZero() {
		status.LastScanAt = lastScan.UTC().Format(time.RFC3339)
	}
	if !s.IsEnabled() || s.db == nil {
		return status, nil
	}

	var total, gps int
	row := s.db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(CASE WHEN has_gps = 1 THEN 1 ELSE 0 END), 0) FROM photo_index")
	if err := row.Scan(&total, &gps); err != nil {
		return status, fmt.Errorf("query photo counts: %w", err)
	}
	status.TotalPhotos = total
	status.GPSPhotos = gps
	return status, nil
}

func (s *Service) TriggerScan() bool {
	if !s.IsEnabled() {
		return false
	}
	s.mu.Lock()
	if s.scanning {
		s.mu.Unlock()
		return false
	}
	s.scanning = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.scanning = false
			s.lastScanAt = time.Now()
			s.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := s.scanDirectory(ctx); err != nil {
			s.logger.Error("photo scan failed", "error", err)
		}
	}()
	return true
}

func (s *Service) ScanSync(ctx context.Context) error {
	if !s.IsEnabled() {
		return errors.New("photo service disabled: root dir not set")
	}
	s.mu.Lock()
	if s.scanning {
		s.mu.Unlock()
		return errors.New("photo scan already in progress")
	}
	s.scanning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.scanning = false
		s.lastScanAt = time.Now()
		s.mu.Unlock()
	}()

	return s.scanDirectory(ctx)
}

type cachedMeta struct {
	id              string
	fileSize        int64
	modTime         int64
	metadataVersion int
}

func (s *Service) scanDirectory(ctx context.Context) error {
	if s.db == nil || s.rootDir == "" {
		return nil
	}

	cached := make(map[string]cachedMeta)
	rows, err := s.db.QueryContext(ctx, "SELECT id, file_path, file_size, mod_time, metadata_version FROM photo_index")
	if err == nil {
		for rows.Next() {
			var id, path string
			var size, mtime int64
			var metadataVersion int
			if err := rows.Scan(&id, &path, &size, &mtime, &metadataVersion); err == nil {
				cached[path] = cachedMeta{id: id, fileSize: size, modTime: mtime, metadataVersion: metadataVersion}
			}
		}
		rows.Close()
	}

	discovered := make(map[string]struct{})
	validExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
		".webp": true, ".tiff": true, ".tif": true,
		".heic": true, ".heif": true,
	}

	now := time.Now().UTC().Format(time.RFC3339)
	err = filepath.WalkDir(s.rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !validExts[ext] {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		cleanPath := filepath.Clean(path)
		discovered[cleanPath] = struct{}{}

		if cm, ok := cached[cleanPath]; ok {
			if cm.fileSize == info.Size() && cm.modTime == info.ModTime().Unix() && cm.metadataVersion == photoMetadataVersion {
				return nil
			}
		}

		id := generatePhotoID(s.rootDir, cleanPath)
		s.indexPhotoFile(ctx, id, cleanPath, d.Name(), info, now)
		return nil
	})

	if err != nil {
		return err
	}

	for oldPath, meta := range cached {
		if _, ok := discovered[oldPath]; !ok {
			_, _ = s.db.ExecContext(ctx, "DELETE FROM photo_index WHERE id = ?", meta.id)
			s.purgeCachedRenditions(meta.id)
		}
	}

	return nil
}

func (s *Service) indexPhotoFile(ctx context.Context, id, path, fileName string, info fs.FileInfo, now string) {
	// The stable ID is derived from the relative path, so invalidate all prior
	// renditions whenever the file is new or its size/modification time changed.
	s.purgeCachedRenditions(id)
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	takenAt := info.ModTime()
	takenAtSource := "file_mod_time"
	takenAtOffsetKnown := true
	var hasGPS bool
	var latWGS, lngWGS, latGCJ, lngGCJ, latBD, lngBD float64
	var capture PhotoCaptureMetadata

	meta, err := ExtractMetadata(file)
	if err == nil && meta != nil {
		if !meta.TakenAt.IsZero() {
			takenAt = meta.TakenAt
			takenAtSource = meta.TakenAtSource
			takenAtOffsetKnown = meta.TakenAtOffsetKnown
		}
		if meta.HasGPS {
			hasGPS = true
			latWGS = meta.Latitude
			lngWGS = meta.Longitude
			latGCJ, lngGCJ = WGS84ToGCJ02(latWGS, lngWGS)
			latBD, lngBD = WGS84ToBD09LL(latWGS, lngWGS)
		}
		capture = PhotoCaptureMetadata{
			CameraMake:     meta.CameraMake,
			CameraModel:    meta.CameraModel,
			LensModel:      meta.LensModel,
			ExposureTimeS:  meta.ExposureTimeS,
			FNumber:        meta.FNumber,
			ISO:            meta.ISO,
			FocalLengthMM:  meta.FocalLengthMM,
			ExposureBiasEV: meta.ExposureBiasEV,
		}
	}

	gpsFlag := 0
	if hasGPS {
		gpsFlag = 1
	}
	takenAtOffsetKnownInt := 0
	if takenAtOffsetKnown {
		takenAtOffsetKnownInt = 1
	}

	query := "INSERT INTO photo_index (id, file_path, file_name, file_size, mod_time, taken_at, taken_at_source, taken_at_offset_known, has_gps, lat_wgs84, lng_wgs84, lat_gcj02, lng_gcj02, lat_bd09ll, lng_bd09ll, camera_make, camera_model, lens_model, exposure_time_s, f_number, iso, focal_length_mm, exposure_bias_ev, metadata_version, created_at, updated_at) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) " +
		"ON CONFLICT(id) DO UPDATE SET file_path=excluded.file_path, file_name=excluded.file_name, file_size=excluded.file_size, mod_time=excluded.mod_time, taken_at=excluded.taken_at, taken_at_source=excluded.taken_at_source, taken_at_offset_known=excluded.taken_at_offset_known, has_gps=excluded.has_gps, lat_wgs84=excluded.lat_wgs84, lng_wgs84=excluded.lng_wgs84, lat_gcj02=excluded.lat_gcj02, lng_gcj02=excluded.lng_gcj02, lat_bd09ll=excluded.lat_bd09ll, lng_bd09ll=excluded.lng_bd09ll, camera_make=excluded.camera_make, camera_model=excluded.camera_model, lens_model=excluded.lens_model, exposure_time_s=excluded.exposure_time_s, f_number=excluded.f_number, iso=excluded.iso, focal_length_mm=excluded.focal_length_mm, exposure_bias_ev=excluded.exposure_bias_ev, metadata_version=excluded.metadata_version, updated_at=excluded.updated_at"

	_, _ = s.db.ExecContext(ctx, query,
		id, path, fileName, info.Size(), info.ModTime().Unix(),
		takenAt.UTC().Format(time.RFC3339), takenAtSource, takenAtOffsetKnownInt, gpsFlag,
		latWGS, lngWGS, latGCJ, lngGCJ, latBD, lngBD,
		capture.CameraMake, capture.CameraModel, capture.LensModel, capture.ExposureTimeS, capture.FNumber, capture.ISO, capture.FocalLengthMM, capture.ExposureBiasEV, photoMetadataVersion,
		now, now,
	)
}

func generatePhotoID(rootDir, fullPath string) string {
	rel, err := filepath.Rel(rootDir, fullPath)
	if err != nil {
		rel = fullPath
	}
	rel = filepath.ToSlash(rel)
	h := sha256.Sum256([]byte(rel))
	return hex.EncodeToString(h[:8])
}

func (s *Service) GetAtlasPhotos(ctx context.Context, crs string) ([]PhotoAtlasItem, error) {
	return s.GetAtlasPhotosWithMatching(ctx, crs, false, DefaultPhotoMatchWindowMinutes)
}

type photoAtlasRecord struct {
	item               PhotoAtlasItem
	filePath           string
	takenAt            time.Time
	takenAtSource      string
	takenAtOffsetKnown bool
	hasGPS             bool
	latWGS84           float64
	lngWGS84           float64
	cameraMake         string
	cameraModel        string
}

func (s *Service) GetAtlasPhotosWithMatching(ctx context.Context, crs string, includeInferred bool, matchWindowMinutes int) ([]PhotoAtlasItem, error) {
	if s.db == nil || !s.IsEnabled() {
		return []PhotoAtlasItem{}, nil
	}
	if !ValidPhotoMatchWindowMinutes(matchWindowMinutes) {
		matchWindowMinutes = DefaultPhotoMatchWindowMinutes
	}
	latCol, lngCol := "lat_gcj02", "lng_gcj02"
	if strings.EqualFold(crs, "bd09ll") {
		latCol, lngCol = "lat_bd09ll", "lng_bd09ll"
	}
	query := fmt.Sprintf("SELECT id, file_path, file_name, taken_at, mod_time, COALESCE(taken_at_source, 'unknown'), taken_at_offset_known, has_gps, COALESCE(lat_wgs84, 0), COALESCE(lng_wgs84, 0), COALESCE(%s, 0), COALESCE(%s, 0), COALESCE(camera_make, ''), COALESCE(camera_model, ''), COALESCE(lens_model, ''), exposure_time_s, f_number, iso, focal_length_mm, exposure_bias_ev FROM photo_index", latCol, lngCol)
	if !includeInferred {
		query += " WHERE has_gps = 1"
	}
	query += " ORDER BY taken_at ASC, id ASC"
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query atlas photos: %w", err)
	}
	defer rows.Close()
	var records []photoAtlasRecord
	for rows.Next() {
		var record photoAtlasRecord
		var takenAtText string
		var modTime int64
		var offsetKnownInt, hasGPSInt int
		var mapLat, mapLng float64
		var capture PhotoCaptureMetadata
		if err := rows.Scan(&record.item.ID, &record.filePath, &record.item.FileName, &takenAtText, &modTime, &record.takenAtSource, &offsetKnownInt, &hasGPSInt, &record.latWGS84, &record.lngWGS84, &mapLat, &mapLng, &record.cameraMake, &record.cameraModel, &capture.LensModel, &capture.ExposureTimeS, &capture.FNumber, &capture.ISO, &capture.FocalLengthMM, &capture.ExposureBiasEV); err != nil {
			continue
		}
		record.takenAt, _ = time.Parse(time.RFC3339, takenAtText)
		record.takenAtOffsetKnown = offsetKnownInt == 1
		record.hasGPS = hasGPSInt == 1
		record.item.TakenAt = takenAtText
		record.item.CacheVersion = modTime
		record.item.Lat, record.item.Lng = mapLat, mapLng
		record.item.ThumbURL = "/api/v1/photos/" + record.item.ID + "/thumbnail"
		record.item.PreviewURL = "/api/v1/photos/" + record.item.ID + "/preview"
		capture.CameraMake, capture.CameraModel = record.cameraMake, record.cameraModel
		if capture.HasValues() {
			record.item.Capture = &capture
		}
		if record.hasGPS {
			record.item.LocationSource = "gps"
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read atlas photos: %w", err)
	}
	if len(records) == 0 {
		return []PhotoAtlasItem{}, nil
	}
	anchors := make(map[string][]photoAtlasRecord)
	if includeInferred {
		root := s.RootDir()
		for _, record := range records {
			if !record.hasGPS || record.takenAt.IsZero() || !validCaptureTimeSource(record.takenAtSource) {
				continue
			}
			key := photoMatchGroupKey(root, record)
			if key != "" {
				anchors[key] = append(anchors[key], record)
			}
		}
		for key := range anchors {
			sort.Slice(anchors[key], func(i, j int) bool {
				if anchors[key][i].takenAt.Equal(anchors[key][j].takenAt) {
					return anchors[key][i].item.ID < anchors[key][j].item.ID
				}
				return anchors[key][i].takenAt.Before(anchors[key][j].takenAt)
			})
		}
	}
	window := time.Duration(matchWindowMinutes) * time.Minute
	result := make([]PhotoAtlasItem, 0, len(records))
	root := ""
	if includeInferred {
		root = s.RootDir()
	}
	for _, record := range records {
		if record.hasGPS {
			result = append(result, record.item)
			continue
		}
		if !includeInferred || record.takenAt.IsZero() || !validCaptureTimeSource(record.takenAtSource) {
			continue
		}
		key := photoMatchGroupKey(root, record)
		group := anchors[key]
		anchor, delta, ok := nearestStableGPSAnchor(record, group, window)
		if !ok {
			continue
		}
		item := record.item
		item.Lat, item.Lng = anchor.item.Lat, anchor.item.Lng
		item.LocationSource = "time_match"
		item.MatchedPhotoID = anchor.item.ID
		item.MatchDeltaSeconds = int64(math.Round(math.Abs(delta.Seconds())))
		result = append(result, item)
	}
	return result, nil
}

func validCaptureTimeSource(source string) bool {
	return source == "exif_original" || source == "exif_digitized"
}

func photoMatchGroupKey(root string, record photoAtlasRecord) string {
	makeName, model := strings.ToLower(strings.TrimSpace(record.cameraMake)), strings.ToLower(strings.TrimSpace(record.cameraModel))
	if root == "" || makeName == "" || model == "" || record.filePath == "" || !validCaptureTimeSource(record.takenAtSource) {
		return ""
	}
	relDir, err := filepath.Rel(root, filepath.Dir(record.filePath))
	if err != nil {
		return ""
	}
	relDir = filepath.ToSlash(filepath.Clean(relDir))
	return relDir + "\x1f" + makeName + "\x1f" + model + "\x1f" + record.takenAtSource + "\x1f" + fmt.Sprint(record.takenAtOffsetKnown)
}

func nearestStableGPSAnchor(target photoAtlasRecord, anchors []photoAtlasRecord, window time.Duration) (photoAtlasRecord, time.Duration, bool) {
	if len(anchors) == 0 {
		return photoAtlasRecord{}, 0, false
	}
	index := sort.Search(len(anchors), func(i int) bool { return !anchors[i].takenAt.Before(target.takenAt) })
	var before, after *photoAtlasRecord
	if index > 0 {
		before = &anchors[index-1]
	}
	if index < len(anchors) {
		after = &anchors[index]
	}
	var beforeDelta, afterDelta time.Duration
	beforeValid, afterValid := false, false
	if before != nil {
		beforeDelta = target.takenAt.Sub(before.takenAt)
		beforeValid = beforeDelta <= window
	}
	if after != nil {
		afterDelta = after.takenAt.Sub(target.takenAt)
		afterValid = afterDelta <= window
	}
	if !beforeValid && !afterValid {
		return photoAtlasRecord{}, 0, false
	}
	if beforeValid && afterValid && photoDistanceMeters(*before, *after) > 300 {
		return photoAtlasRecord{}, 0, false
	}
	if beforeValid && (!afterValid || beforeDelta <= afterDelta) {
		return *before, beforeDelta, true
	}
	return *after, afterDelta, true
}

func photoDistanceMeters(a, b photoAtlasRecord) float64 {
	const earthRadiusMeters = 6371000.0
	lat1, lat2 := a.latWGS84*math.Pi/180, b.latWGS84*math.Pi/180
	dLat := lat2 - lat1
	dLng := (b.lngWGS84 - a.lngWGS84) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusMeters * math.Asin(math.Sqrt(math.Min(1, h)))
}

func (s *Service) GetPhotoByID(ctx context.Context, id string) (*PhotoRecord, error) {
	if s.db == nil {
		return nil, errors.New("database not available")
	}
	row := s.db.QueryRowContext(ctx, "SELECT id, file_path, file_name, file_size, mod_time, taken_at, taken_at_source, taken_at_offset_known, has_gps, lat_wgs84, lng_wgs84, lat_gcj02, lng_gcj02, lat_bd09ll, lng_bd09ll, camera_make, camera_model, lens_model, exposure_time_s, f_number, iso, focal_length_mm, exposure_bias_ev, metadata_version, created_at, updated_at FROM photo_index WHERE id = ?", id)
	var p PhotoRecord
	var hasGPSInt, offsetKnownInt int
	var takenAtStr, createdAtStr, updatedAtStr string
	err := row.Scan(&p.ID, &p.FilePath, &p.FileName, &p.FileSize, &p.ModTime, &takenAtStr, &p.TakenAtSource, &offsetKnownInt, &hasGPSInt, &p.LatWGS84, &p.LngWGS84, &p.LatGCJ02, &p.LngGCJ02, &p.LatBD09LL, &p.LngBD09LL, &p.Capture.CameraMake, &p.Capture.CameraModel, &p.Capture.LensModel, &p.Capture.ExposureTimeS, &p.Capture.FNumber, &p.Capture.ISO, &p.Capture.FocalLengthMM, &p.Capture.ExposureBiasEV, &p.MetadataVersion, &createdAtStr, &updatedAtStr)
	if err != nil {
		return nil, err
	}
	p.HasGPS = hasGPSInt == 1
	p.TakenAtOffsetKnown = offsetKnownInt == 1
	p.TakenAt, _ = time.Parse(time.RFC3339, takenAtStr)
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	return &p, nil
}

const (
	DefaultThumbnailSize = 120
	PreviewSmallEdge     = 960
	PreviewLargeEdge     = 1600
)

func isSupportedPreviewEdge(maxEdge int) bool {
	return maxEdge == PreviewSmallEdge || maxEdge == PreviewLargeEdge
}

func (s *Service) GetThumbnail(ctx context.Context, id string, size int) ([]byte, error) {
	if size <= 0 {
		size = DefaultThumbnailSize
	}
	photo, err := s.GetPhotoByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.getCachedRendition(photo, "thumbnails", size, GenerateThumbnail)
}

// GetPreview returns an aspect-ratio-preserving JPEG rendition for the Lightbox.
// Only fixed 960px and 1600px long-edge tiers are supported to bound cache growth.
func (s *Service) GetPreview(ctx context.Context, id string, maxEdge int) ([]byte, error) {
	if maxEdge == 0 {
		maxEdge = PreviewLargeEdge
	}
	if !isSupportedPreviewEdge(maxEdge) {
		return nil, fmt.Errorf("unsupported preview max edge: %d", maxEdge)
	}
	photo, err := s.GetPhotoByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.getCachedRendition(photo, "previews", maxEdge, GeneratePreview)
}

type renditionGenerator func(srcPath, destPath string, size int) error

func (s *Service) getCachedRendition(photo *PhotoRecord, kind string, size int, generate renditionGenerator) ([]byte, error) {
	if s.cacheDir == "" {
		return FallbackThumbnailBytes(), nil
	}
	cachePath := s.renditionCachePath(kind, photo.ID, photo.ModTime, size)
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		return data, nil
	}
	if err := generate(photo.FilePath, cachePath, size); err == nil {
		if data, readErr := os.ReadFile(cachePath); readErr == nil && len(data) > 0 {
			return data, nil
		}
	}
	// Unsupported formats (for example HEIC without a decoder) retain the existing
	// graceful image fallback. The UI exposes an explicit original-file action.
	return FallbackThumbnailBytes(), nil
}

func (s *Service) renditionCachePath(kind, id string, modTime int64, size int) string {
	return filepath.Join(s.cacheDir, kind, fmt.Sprintf("%s_%d_%d.jpg", id, modTime, size))
}

func (s *Service) purgeCachedRenditions(id string) {
	if s.cacheDir == "" || id == "" {
		return
	}
	for _, kind := range []string{"thumbnails", "previews"} {
		dir := filepath.Join(s.cacheDir, kind)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), id+"_") {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}
}

func (s *Service) GetPhotoFilePath(ctx context.Context, id string) (string, error) {
	photo, err := s.GetPhotoByID(ctx, id)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(photo.FilePath)
	if s.rootDir != "" {
		if !strings.HasPrefix(clean, s.rootDir) {
			return "", errors.New("access denied: outside root photo directory")
		}
	}
	if _, err := os.Stat(clean); err != nil {
		return "", err
	}
	return clean, nil
}
