package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
)

type AtlasStopSummary struct {
	ID       string                 `json:"id"`
	Sequence int                    `json:"sequence"`
	Title    string                 `json:"title"`
	DayIndex int                    `json:"day_index"`
	Point    *journeymaps.GeoPoint  `json:"point,omitempty"`
}

type AtlasLegSummary struct {
	ID         string     `json:"id"`
	FromStopID string     `json:"from_stop_id"`
	ToStopID   string     `json:"to_stop_id"`
	Mode       string     `json:"mode,omitempty"`
	DistanceM  int        `json:"distance_m,omitempty"`
	DurationS  int        `json:"duration_s,omitempty"`
	Geometry   [][]float64 `json:"geometry,omitempty"`
	CRS        string     `json:"crs,omitempty"`
}

type AtlasTripItem struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	StartDate   string             `json:"start_date"`
	EndDate     string             `json:"end_date"`
	DaysCount   int                `json:"days_count"`
	StopsCount  int                `json:"stops_count"`
	DistanceM   int                `json:"distance_m"`
	DurationS   int                `json:"duration_s"`
	KeyStops    []AtlasStopSummary `json:"key_stops"`
	Legs        []AtlasLegSummary  `json:"legs"`
}

type AtlasSummaryResponse struct {
	TotalTrips     int             `json:"total_trips"`
	TotalDays      int             `json:"total_days"`
	TotalStops     int             `json:"total_stops"`
	TotalDistanceM int             `json:"total_distance_m"`
	TotalDurationS int             `json:"total_duration_s"`
	Trips          []AtlasTripItem `json:"trips"`
}

func (s *Server) getAtlasSummary(w http.ResponseWriter, r *http.Request) {
	records, err := s.trips.List(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}

	preferredProvider := string(journeymaps.ProviderAMap)
	if defaultP, err := s.defaultMapProviderFor(r.Context()); err == nil && defaultP != "" {
		preferredProvider = string(defaultP)
	}

	atlasItems := make([]AtlasTripItem, 0, len(records))
	totalDays := 0
	totalStops := 0
	totalDistanceM := 0
	totalDurationS := 0

	for _, record := range records {
		var trip domain.Trip
		if err := json.Unmarshal(record.Document, &trip); err != nil {
			continue
		}

		// 默认显示在足迹漫游中，除非显式设置 false
		if trip.ShowInAtlas != nil && !*trip.ShowInAtlas {
			continue
		}

		tripDistanceM := 0
		tripDurationS := 0
		keyStops := make([]AtlasStopSummary, 0)
		legs := make([]AtlasLegSummary, 0)

		tripStopsCount := 0
		for dayIdx, day := range trip.Days {
			tripStopsCount += len(day.Stops)
			for sIdx, stop := range day.Stops {
				// 提取代表性关键停靠点：每日首末点或行程首末点
				isKeyStop := sIdx == 0 || sIdx == len(day.Stops)-1
				if isKeyStop {
					var pt *journeymaps.GeoPoint
					if p, err := extractPointFromLocation(stop.Location); err == nil {
						pt = &p
					}
					keyStops = append(keyStops, AtlasStopSummary{
						ID:       stop.ID,
						Sequence: stop.Sequence,
						Title:    stop.Title,
						DayIndex: dayIdx + 1,
						Point:    pt,
					})
				}
			}

			for _, leg := range day.Legs {
				// 优选与 preferredProvider 匹配的路线几何
				var chosenSnapshot *domain.RouteSnapshot
				for _, snap := range leg.Snapshots {
					if snap.Provider == preferredProvider && len(snap.Geometry) > 1 {
						chosenSnapshot = &snap
						break
					}
				}
				if chosenSnapshot == nil && len(leg.Snapshots) > 0 {
					for _, snap := range leg.Snapshots {
						if len(snap.Geometry) > 1 {
							chosenSnapshot = &snap
							break
						}
					}
				}

				if chosenSnapshot != nil {
					tripDistanceM += chosenSnapshot.DistanceM
					tripDurationS += chosenSnapshot.DurationS
					legs = append(legs, AtlasLegSummary{
						ID:         leg.ID,
						FromStopID: leg.FromStopID,
						ToStopID:   leg.ToStopID,
						Mode:       leg.Mode,
						DistanceM:  chosenSnapshot.DistanceM,
						DurationS:  chosenSnapshot.DurationS,
						Geometry:   chosenSnapshot.Geometry,
						CRS:        chosenSnapshot.CoordinateSystem,
					})
				}
			}
		}

		totalDays += len(trip.Days)
		totalStops += tripStopsCount
		totalDistanceM += tripDistanceM
		totalDurationS += tripDurationS

		atlasItems = append(atlasItems, AtlasTripItem{
			ID:         record.ID,
			Title:      record.Title,
			StartDate:  record.StartDate,
			EndDate:    record.EndDate,
			DaysCount:  len(trip.Days),
			StopsCount: tripStopsCount,
			DistanceM:  tripDistanceM,
			DurationS:  tripDurationS,
			KeyStops:   keyStops,
			Legs:       legs,
		})
	}

	// 按行程起始日期倒序排列（最新行程排在前）
	sort.Slice(atlasItems, func(i, j int) bool {
		return atlasItems[i].StartDate > atlasItems[j].StartDate
	})

	response := AtlasSummaryResponse{
		TotalTrips:     len(atlasItems),
		TotalDays:      totalDays,
		TotalStops:     totalStops,
		TotalDistanceM: totalDistanceM,
		TotalDurationS: totalDurationS,
		Trips:          atlasItems,
	}

	writeJSON(w, http.StatusOK, response)
}

func extractPointFromLocation(raw json.RawMessage) (journeymaps.GeoPoint, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return journeymaps.GeoPoint{}, errors.New("location required")
	}
	var loc struct {
		Preferred   string                          `json:"preferred"`
		Coordinates map[string]journeymaps.GeoPoint `json:"coordinates"`
		Lat         *float64                        `json:"lat"`
		Lng         *float64                        `json:"lng"`
		CRS         string                          `json:"crs"`
	}
	if err := json.Unmarshal(raw, &loc); err != nil {
		return journeymaps.GeoPoint{}, err
	}
	for _, key := range []string{loc.Preferred, "gcj02", "bd09ll", "wgs84"} {
		if pt, ok := loc.Coordinates[key]; ok {
			return pt, nil
		}
	}
	if loc.Lat != nil && loc.Lng != nil {
		crs := loc.CRS
		if crs == "" {
			crs = loc.Preferred
		}
		if crs == "" {
			crs = "gcj02"
		}
		return journeymaps.GeoPoint{Lat: *loc.Lat, Lng: *loc.Lng, CRS: journeymaps.CoordinateSystem(crs)}, nil
	}
	return journeymaps.GeoPoint{}, errors.New("location required")
}
