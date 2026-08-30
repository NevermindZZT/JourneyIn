package application

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	journeymaps "journeyin/internal/maps"
)

type concurrentMapProvider struct {
	fakePlanningProvider
	inFlight  atomic.Int32
	maxFlight atomic.Int32
}

func (p *concurrentMapProvider) enter(ctx context.Context) error {
	current := p.inFlight.Add(1)
	defer func() {
		if current == 1 {
			p.maxFlight.CompareAndSwap(0, 1)
		}
	}()
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
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *concurrentMapProvider) leave() {
	p.inFlight.Add(-1)
}

func (p *concurrentMapProvider) SearchPOI(ctx context.Context, query, region string, page, pageSize int) (journeymaps.POISearchResult, error) {
	if err := p.enter(ctx); err != nil {
		return journeymaps.POISearchResult{}, err
	}
	defer p.leave()
	return p.fakePlanningProvider.SearchPOI(ctx, query, region, page, pageSize)
}

func (p *concurrentMapProvider) Route(ctx context.Context, request journeymaps.RouteRequest) (journeymaps.RouteSnapshot, error) {
	if err := p.enter(ctx); err != nil {
		return journeymaps.RouteSnapshot{}, err
	}
	defer p.leave()
	return p.fakePlanningProvider.Route(ctx, request)
}

func TestMapServiceSerializesAllSameProviderRequests(t *testing.T) {
	service := testService(t)
	provider := &concurrentMapProvider{}
	mapService := NewMapService(service.store, journeymaps.NewRegistry(provider), 8, 0)
	const requestCount = 4
	errorsCh := make(chan error, requestCount*2)
	var group sync.WaitGroup
	for index := 0; index < requestCount; index++ {
		index := index
		group.Add(2)
		go func() {
			defer group.Done()
			_, err := mapService.SearchPOI(context.Background(), "fake", fmt.Sprintf("query-%d", index), "region", 1, 10)
			errorsCh <- err
		}()
		go func() {
			defer group.Done()
			_, err := mapService.Route(context.Background(), "fake", journeymaps.RouteRequest{
				Origin:      journeymaps.GeoPoint{Lat: 30.20, Lng: 120.10 + float64(index)*0.001, CRS: journeymaps.CRSBD09LL},
				Destination: journeymaps.GeoPoint{Lat: 30.21, Lng: 120.11 + float64(index)*0.001, CRS: journeymaps.CRSBD09LL},
				Mode:        journeymaps.ModeWalking,
			})
			errorsCh <- err
		}()
	}
	group.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := provider.maxFlight.Load(); got != 1 {
		t.Fatalf("maximum concurrent same-provider calls=%d, want 1", got)
	}
}
