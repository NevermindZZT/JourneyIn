package maps

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type baiduRoundTripFunc func(*http.Request) (*http.Response, error)

func (f baiduRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testBaiduRouteRequest() RouteRequest {
	return RouteRequest{
		Origin:      GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSBD09LL},
		Destination: GeoPoint{Lat: 30.26, Lng: 120.16, CRS: CRSBD09LL},
		Mode:        ModeWalking,
	}
}

func TestBaiduProviderRetriesConcurrencyStatus(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":401,"message":"concurrency limited","result":{}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"result":{"routes":[{"distance":100,"duration":200,"steps":[{"path":"120.150000,30.250000;120.151000,30.251000"}]}]}}`))
	}))
	defer server.Close()

	provider := NewBaiduProvider(BaiduConfig{ServerAK: "test-ak", BaseURL: server.URL})
	if _, err := provider.Route(context.Background(), testBaiduRouteRequest()); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("request attempts=%d, want 2", got)
	}
}

func TestBaiduProviderDoesNotRetryDailyQuotaStatus(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":4,"message":"daily quota exceeded","result":{}}`))
	}))
	defer server.Close()

	provider := NewBaiduProvider(BaiduConfig{ServerAK: "test-ak", BaseURL: server.URL})
	_, err := provider.Route(context.Background(), testBaiduRouteRequest())
	if !errors.Is(err, ErrProviderQuotaExceeded) {
		t.Fatalf("error=%v, want quota classification", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("request attempts=%d, want 1", got)
	}
}

func TestBaiduProviderRetriesTransientNetworkErrorWithoutLeakingAK(t *testing.T) {
	const secret = "test-secret-ak"
	var calls atomic.Int32
	transportError := &net.DNSError{Name: "api.map.baidu.com", Err: "server misbehaving", IsTemporary: true}
	client := &http.Client{Transport: baiduRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, &url.Error{Op: "Get", URL: request.URL.String(), Err: transportError}
	})}
	provider := NewBaiduProvider(BaiduConfig{ServerAK: secret, HTTPClient: client})

	_, err := provider.Route(context.Background(), testBaiduRouteRequest())
	if !errors.Is(err, ErrProviderTemporary) {
		t.Fatalf("error=%v, want temporary classification", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "ak=") {
		t.Fatalf("error leaked request credential: %v", err)
	}
	if got := calls.Load(); got != baiduMaxRequestAttempts {
		t.Fatalf("request attempts=%d, want %d", got, baiduMaxRequestAttempts)
	}
}

func TestBaiduProviderPreservesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := NewBaiduProvider(BaiduConfig{ServerAK: "test-ak"})

	_, err := provider.Route(ctx, testBaiduRouteRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context cancellation", err)
	}
	if errors.Is(err, ErrProviderTemporary) {
		t.Fatalf("caller cancellation must not be classified as transient: %v", err)
	}
}

func TestBaiduProviderDefaultTransportLimitsHostConnections(t *testing.T) {
	provider := NewBaiduProvider(BaiduConfig{ServerAK: "test-ak"})
	transport, ok := provider.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type=%T, want *http.Transport", provider.client.Transport)
	}
	if transport.MaxConnsPerHost != 1 || transport.MaxIdleConnsPerHost != 1 {
		t.Fatalf("host connection limits=(%d,%d), want (1,1)", transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost)
	}
}

func TestBaiduProviderRetryBudgetIsBoundedByRequestTimeout(t *testing.T) {
	client := &http.Client{Transport: baiduRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: request.URL.String(), Err: &net.DNSError{Name: "api.map.baidu.com", Err: "server misbehaving", IsTemporary: true}}
	})}
	provider := NewBaiduProvider(BaiduConfig{ServerAK: "test-ak", HTTPClient: client, RequestTimeout: 20 * time.Millisecond})
	started := time.Now()
	_, err := provider.Route(context.Background(), testBaiduRouteRequest())
	if !errors.Is(err, ErrProviderTemporary) {
		t.Fatalf("error=%v, want temporary classification", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("retry budget exceeded request timeout: %s", elapsed)
	}
}
