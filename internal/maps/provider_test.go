package maps

import (
	"context"
	"errors"
	"testing"
)

func TestUnavailableProviderMakesDowngradeExplicit(t *testing.T) {
	provider := NewUnavailableProvider(ProviderBaidu)
	_, err := provider.Route(context.Background(), RouteRequest{Mode: ModeWalking})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected provider unavailable, got %v", err)
	}
	_, err = provider.NavigationURL(NavTarget{Name: "西湖", Location: GeoPoint{Lat: 30.2, Lng: 120.1, CRS: CRSGCJ02}}, ModeWalking, PlatformWeb)
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected provider unavailable, got %v", err)
	}
}

func TestRegistryKeepsProviderBoundary(t *testing.T) {
	registry := NewRegistry(NewUnavailableProvider(ProviderBaidu), NewUnavailableProvider(ProviderAMap))
	if _, ok := registry.Get(ProviderBaidu); !ok {
		t.Fatal("baidu provider missing")
	}
	if _, ok := registry.Get(ProviderAMap); !ok {
		t.Fatal("amap provider missing")
	}
}

func TestValidateNavigationURL(t *testing.T) {
	if err := ValidateNavigationURL("https://uri.amap.com/navigation"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNavigationURL("javascript:alert(1)"); err == nil {
		t.Fatal("expected unsafe scheme to fail")
	}
}
