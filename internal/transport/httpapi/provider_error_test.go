package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	journeymaps "journeyin/internal/maps"
)

func TestWritePlanningErrorClassifiesProviderFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "rate limited", err: fmt.Errorf("waiting: %w", journeymaps.ErrProviderRateLimited), statusCode: http.StatusTooManyRequests, code: "provider_rate_limited"},
		{name: "quota exceeded", err: fmt.Errorf("daily: %w", journeymaps.ErrProviderQuotaExceeded), statusCode: http.StatusTooManyRequests, code: "provider_quota_exceeded"},
		{name: "temporary", err: fmt.Errorf("dns: %w", journeymaps.ErrProviderTemporary), statusCode: http.StatusServiceUnavailable, code: "provider_temporary"},
		{name: "unauthorized", err: fmt.Errorf("invalid key: %w", journeymaps.ErrProviderUnauthorized), statusCode: http.StatusBadGateway, code: "provider_unauthorized"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writePlanningError(response, test.err)
			if response.Code != test.statusCode {
				t.Fatalf("status=%d, want %d", response.Code, test.statusCode)
			}
			var payload map[string]map[string]any
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if got := payload["error"]["code"]; got != test.code {
				t.Fatalf("code=%v, want %s", got, test.code)
			}
		})
	}
}
