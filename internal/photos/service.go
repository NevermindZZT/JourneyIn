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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PhotoRecord struct {
	ID        string    `json:"id"`
	FilePath  string    `json:"file_path"`
	FileName  string    `json:"file_name"`
	FileSize  int64     `json:"file_size"`
	ModTime   int64     `json:"mod_time"`
	TakenAt   time.Time `json:"taken_at"`
	HasGPS    bool      `json:"has_gps"`
	LatWGS84  float64   `json:"lat_wgs84,omitempty"`
	LngWGS84  float64   `json:"lng_wgs84,omitempty"`
	LatGCJ02  float64   `json:"lat_gcj02,omitempty"`
	LngGCJ02  float64   `json:"lng_gcj02,omitempty"`
	LatBD09LL float64   `json:"lat_bd09ll,omitempty"`
	LngBD09LL float64   `json:"lng_bd09ll,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PhotoAtlasItem struct {
	ID       string  `json:"id"`
	FileName string  `json:"file_name"`
	TakenAt  string  `json:"taken_at"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	ThumbURL string  `json:"thumb_url"`
}

type PhotoStatus struct {
	Enabled     bool   `json:"enabled"`
	RootDir     string `json:"root_dir"`
	TotalPhotos int    `json:"total_photos"`
	GPSPhotos   int    `json:"gps_photos"`
	Scanning    bool   `json:"scanning"`
	LastScanAt  string `json:"last_scan_at,omitempty"`
}

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
	id       string
	fileSize int64
	modTime  int64
}

func (s *Service) scanDirectory(ctx context.Context) error {
	if s.db == nil || s.rootDir == "" {
		return nil
	}

	cached := make(map[string]cachedMeta)
	rows, err := s.db.QueryContext(ctx, "SELECT id, file_path, file_size, mod_time FROM photo_index")
	if err == nil {
		for rows.Next() {
			var id, path string
			var size, mtime int64
			if err := rows.Scan(&id, &path, &size, &mtime); err == nil {
				cached[path] = cachedMeta{id: id, fileSize: size, modTime: mtime}
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
			if cm.fileSize == info.Size() && cm.modTime == info.ModTime().Unix() {
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
		}
	}

	return nil
}

func (s *Service) indexPhotoFile(ctx context.Context, id, path, fileName string, info fs.FileInfo, now string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	takenAt := info.ModTime()
	var hasGPS bool
	var latWGS, lngWGS, latGCJ, lngGCJ, latBD, lngBD float64

	meta, err := ExtractMetadata(file)
	if err == nil && meta != nil {
		if !meta.TakenAt.IsZero() {
			takenAt = meta.TakenAt
		}
		if meta.HasGPS {
			hasGPS = true
			latWGS = meta.Latitude
			lngWGS = meta.Longitude
			latGCJ, lngGCJ = WGS84ToGCJ02(latWGS, lngWGS)
			latBD, lngBD = WGS84ToBD09LL(latWGS, lngWGS)
		}
	}

	gpsFlag := 0
	if hasGPS {
		gpsFlag = 1
	}

	query := "INSERT INTO photo_index (id, file_path, file_name, file_size, mod_time, taken_at, has_gps, lat_wgs84, lng_wgs84, lat_gcj02, lng_gcj02, lat_bd09ll, lng_bd09ll, created_at, updated_at) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) " +
		"ON CONFLICT(id) DO UPDATE SET file_path=excluded.file_path, file_name=excluded.file_name, file_size=excluded.file_size, mod_time=excluded.mod_time, taken_at=excluded.taken_at, has_gps=excluded.has_gps, lat_wgs84=excluded.lat_wgs84, lng_wgs84=excluded.lng_wgs84, lat_gcj02=excluded.lat_gcj02, lng_gcj02=excluded.lng_gcj02, lat_bd09ll=excluded.lat_bd09ll, lng_bd09ll=excluded.lng_bd09ll, updated_at=excluded.updated_at"

	_, _ = s.db.ExecContext(ctx, query,
		id, path, fileName, info.Size(), info.ModTime().Unix(),
		takenAt.UTC().Format(time.RFC3339), gpsFlag,
		latWGS, lngWGS, latGCJ, lngGCJ, latBD, lngBD,
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
	if s.db == nil || !s.IsEnabled() {
		return []PhotoAtlasItem{}, nil
	}

	latCol, lngCol := "lat_gcj02", "lng_gcj02"
	if strings.ToLower(crs) == "bd09ll" {
		latCol, lngCol = "lat_bd09ll", "lng_bd09ll"
	}

	query := fmt.Sprintf("SELECT id, file_name, taken_at, %s, %s FROM photo_index WHERE has_gps = 1 ORDER BY taken_at ASC", latCol, lngCol)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query atlas photos: %w", err)
	}
	defer rows.Close()

	var items []PhotoAtlasItem
	for rows.Next() {
		var item PhotoAtlasItem
		if err := rows.Scan(&item.ID, &item.FileName, &item.TakenAt, &item.Lat, &item.Lng); err != nil {
			continue
		}
		item.ThumbURL = "/api/v1/photos/" + item.ID + "/thumbnail"
		items = append(items, item)
	}
	if items == nil {
		items = []PhotoAtlasItem{}
	}
	return items, nil
}

func (s *Service) GetPhotoByID(ctx context.Context, id string) (*PhotoRecord, error) {
	if s.db == nil {
		return nil, errors.New("database not available")
	}
	row := s.db.QueryRowContext(ctx, "SELECT id, file_path, file_name, file_size, mod_time, taken_at, has_gps, lat_wgs84, lng_wgs84, lat_gcj02, lng_gcj02, lat_bd09ll, lng_bd09ll, created_at, updated_at FROM photo_index WHERE id = ?", id)
	var p PhotoRecord
	var hasGPSInt int
	var takenAtStr, createdAtStr, updatedAtStr string
	err := row.Scan(&p.ID, &p.FilePath, &p.FileName, &p.FileSize, &p.ModTime, &takenAtStr, &hasGPSInt, &p.LatWGS84, &p.LngWGS84, &p.LatGCJ02, &p.LngGCJ02, &p.LatBD09LL, &p.LngBD09LL, &createdAtStr, &updatedAtStr)
	if err != nil {
		return nil, err
	}
	p.HasGPS = hasGPSInt == 1
	p.TakenAt, _ = time.Parse(time.RFC3339, takenAtStr)
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	return &p, nil
}

func (s *Service) GetThumbnail(ctx context.Context, id string, size int) ([]byte, error) {
	if size <= 0 {
		size = 120
	}
	if s.cacheDir != "" {
		thumbPath := filepath.Join(s.cacheDir, fmt.Sprintf("%s_%d.jpg", id, size))
		if data, err := os.ReadFile(thumbPath); err == nil && len(data) > 0 {
			return data, nil
		}
	}

	photo, err := s.GetPhotoByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if s.cacheDir != "" {
		thumbPath := filepath.Join(s.cacheDir, fmt.Sprintf("%s_%d.jpg", id, size))
		if err := GenerateThumbnail(photo.FilePath, thumbPath, size); err == nil {
			return os.ReadFile(thumbPath)
		}
	}

	return FallbackThumbnailBytes(), nil
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
