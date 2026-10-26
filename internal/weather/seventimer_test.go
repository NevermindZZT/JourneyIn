package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSevenTimerProvider_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := sevenTimerResponse{
			Product: "astro",
			Init:    "2026092700",
			Dataseries: []sevenTimerPoint{
				{
					Timepoint:    24, // 2026-09-28 00:00 UTC
					Cloudcover:   2,  // 12%
					Seeing:       4,  // 等级4
					Transparency: 2,  // 等级2 (透明)
					Temp2m:       18,
					PrecType:     "none",
				},
				{
					Timepoint:    30, // 2026-09-28 06:00 UTC
					Cloudcover:   3,  // 25%
					Seeing:       4,
					Transparency: 2,
					Temp2m:       24,
					PrecType:     "none",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewSevenTimerProvider()
	p.SetBaseURL(server.URL)

	snap, err := p.Weather(context.Background(), WeatherRequest{
		Location:  GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSWGS84},
		LocalDate: "2026-09-28",
		Timezone:  "UTC",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !snap.Available {
		t.Fatal("expected snap to be available")
	}
	if snap.Provider != Provider7Timer {
		t.Errorf("expected provider 7timer, got %s", snap.Provider)
	}
	if snap.Seeing == nil || *snap.Seeing != 4.0 {
		t.Errorf("expected Seeing 4.0, got %v", snap.Seeing)
	}
	if snap.Transparency == nil || *snap.Transparency != 2.0 {
		t.Errorf("expected Transparency 2.0, got %v", snap.Transparency)
	}
	if snap.CloudCoverPct == nil || *snap.CloudCoverPct < 10 || *snap.CloudCoverPct > 25 {
		t.Errorf("expected CloudCoverPct around 18, got %v", snap.CloudCoverPct)
	}
}
