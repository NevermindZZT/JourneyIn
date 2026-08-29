package maps

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestAMapNavigationAcceptsBD09LLAndConvertsAtProviderBoundary(t *testing.T) {
	point := GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSBD09LL}
	raw, err := NewAMapProvider("journeyin-test").NavigationURL(NavTarget{Name: "西湖", Location: point}, ModeWalking, PlatformWeb)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	expected := bd09llToGCJ02(point)
	prefix := fmt.Sprintf("%.8f,%.8f,", expected.Lng, expected.Lat)
	if got := parsed.Query().Get("to"); !strings.HasPrefix(got, prefix) {
		t.Fatalf("AMap destination=%q, want prefix %q", got, prefix)
	}
	if parsed.Query().Get("coordinate") != "gaode" {
		t.Fatalf("coordinate=%q, want gaode", parsed.Query().Get("coordinate"))
	}
	if err := ValidateNavigationURL(raw); err != nil {
		t.Fatal(err)
	}
}

func TestAMapNavigationAcceptsWGS84InChina(t *testing.T) {
	raw, err := NewAMapProvider("journeyin-test").NavigationURL(NavTarget{Name: "西湖", Location: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSWGS84}}, ModeWalking, PlatformIOS)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "iosamap" || parsed.Query().Get("dlat") == "30.25000000" || parsed.Query().Get("dlon") == "120.15000000" {
		t.Fatalf("WGS84 coordinates were not converted: %s", raw)
	}
	if err := ValidateNavigationURL(raw); err != nil {
		t.Fatal(err)
	}
}

func TestNavigationURLValidationCoversDesktopAndNativeProviders(t *testing.T) {
	baidu, err := NewBaiduProvider(BaiduConfig{}).NavigationURL(NavTarget{Name: "西湖", Location: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSBD09LL}}, ModeWalking, PlatformWeb)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(baidu, "https://api.map.baidu.com/direction?") {
		t.Fatalf("unexpected desktop Baidu URL: %s", baidu)
	}
	if err := ValidateNavigationURL(baidu); err != nil {
		t.Fatal(err)
	}
	amap, err := NewAMapProvider("journeyin-test").NavigationURL(NavTarget{Name: "西湖", Location: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02}}, ModeWalking, PlatformAndroid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(amap, "amapuri://route/plan/?") {
		t.Fatalf("unexpected Android AMap URL: %s", amap)
	}
	if err := ValidateNavigationURL(amap); err != nil {
		t.Fatal(err)
	}
}
