package application

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	journeymaps "journeyin/internal/maps"
)

type concurrentRouteProvider struct {
	fakePlanningProvider
	inFlight  atomic.Int32
	maxFlight atomic.Int32
}

func (p *concurrentRouteProvider) Route(ctx context.Context, request journeymaps.RouteRequest) (journeymaps.RouteSnapshot, error) {
	current := p.inFlight.Add(1)
	defer p.inFlight.Add(-1)
	for {
		previous := p.maxFlight.Load()
		if current <= previous || p.maxFlight.CompareAndSwap(previous, current) {
			break
		}
	}
	timer := time.NewTimer(10 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return journeymaps.RouteSnapshot{}, ctx.Err()
	}
	return p.fakePlanningProvider.Route(ctx, request)
}

func TestMapServiceSerializesRouteProviderRequests(t *testing.T) {
	service := testService(t)
	provider := &concurrentRouteProvider{}
	mapService := NewMapService(service.store, journeymaps.NewRegistry(provider), 8, 0)
	const requestCount = 8
	errorsCh := make(chan error, requestCount)
	var group sync.WaitGroup
	for index := 0; index < requestCount; index++ {
		index := index
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := mapService.Route(context.Background(), "fake", journeymaps.RouteRequest{
				Origin:      journeymaps.GeoPoint{Lat: 30.20, Lng: 120.10 + float64(index)*0.001, CRS: journeymaps.CRSBD09LL},
				Destination: journeymaps.GeoPoint{Lat: 30.21, Lng: 120.11 + float64(index)*0.001, CRS: journeymaps.CRSBD09LL},
				Mode:        journeymaps.ModeWalking,
			})
			if err != nil {
				errorsCh <- err
			}
		}()
	}
	group.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Fatal(err)
	}
	if got := provider.maxFlight.Load(); got != 1 {
		t.Fatalf("maximum concurrent route provider calls=%d, want 1", got)
	}
	if got := provider.routeCalls.Load(); got != requestCount {
		t.Fatalf("route calls=%d, want %d", got, requestCount)
	}
}
