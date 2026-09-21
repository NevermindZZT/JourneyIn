package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"journeyin/internal/photos"
)

func (s *Server) getPhotoStatus(w http.ResponseWriter, r *http.Request) {
	if s.photosService == nil {
		writeJSON(w, http.StatusOK, photos.PhotoStatus{Enabled: false})
		return
	}
	status, err := s.photosService.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "photo_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) syncPhotos(w http.ResponseWriter, r *http.Request) {
	if s.photosService == nil || !s.photosService.IsEnabled() {
		writeError(w, http.StatusBadRequest, "disabled", "photo service is not enabled", nil)
		return
	}
	triggered := s.photosService.TriggerScan()
	writeJSON(w, http.StatusOK, map[string]any{
		"triggered": triggered,
		"message":   "photo scan triggered in background",
	})
}

func (s *Server) getAtlasPhotos(w http.ResponseWriter, r *http.Request) {
	if s.photosService == nil || !s.photosService.IsEnabled() {
		writeJSON(w, http.StatusOK, []photos.PhotoAtlasItem{})
		return
	}
	crs := strings.TrimSpace(r.URL.Query().Get("crs"))
	if crs == "" {
		// 依据系统默认地图 provider 确定默认坐标系
		if defaultP, err := s.defaultMapProviderFor(r.Context()); err == nil && defaultP == "baidu" {
			crs = "bd09ll"
		} else {
			crs = "gcj02"
		}
	}
	items, err := s.photosService.GetAtlasPhotos(r.Context(), crs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getPhotoThumbnail(w http.ResponseWriter, r *http.Request) {
	if s.photosService == nil || !s.photosService.IsEnabled() {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	size := 120
	if sizeParam := r.URL.Query().Get("size"); sizeParam != "" {
		if parsed, err := strconv.Atoi(sizeParam); err == nil && parsed > 0 && parsed <= 600 {
			size = parsed
		}
	}
	thumbBytes, err := s.photosService.GetThumbnail(r.Context(), id, size)
	if err != nil {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(photos.FallbackThumbnailBytes())
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	setVersionedPhotoCacheHeaders(w, r)
	_, _ = w.Write(thumbBytes)
}

func (s *Server) getPhotoPreview(w http.ResponseWriter, r *http.Request) {
	if s.photosService == nil || !s.photosService.IsEnabled() {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	maxEdge := photos.PreviewLargeEdge
	if raw := strings.TrimSpace(r.URL.Query().Get("max_edge")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != photos.PreviewSmallEdge && parsed != photos.PreviewLargeEdge) {
			writeError(w, http.StatusBadRequest, "invalid_preview_size", "preview max_edge must be 960 or 1600", nil)
			return
		}
		maxEdge = parsed
	}
	previewBytes, err := s.photosService.GetPreview(r.Context(), id, maxEdge)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	setVersionedPhotoCacheHeaders(w, r)
	_, _ = w.Write(previewBytes)
}

func setVersionedPhotoCacheHeaders(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("v")) != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
}

func (s *Server) getPhotoFile(w http.ResponseWriter, r *http.Request) {
	if s.photosService == nil || !s.photosService.IsEnabled() {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	filePath, err := s.photosService.GetPhotoFilePath(r.Context(), id)
	if err != nil {
		if errors.Is(err, errors.New("access denied: outside root photo directory")) {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filePath)
}
