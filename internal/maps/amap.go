package maps

import (
	"fmt"
	"net/url"
)

type AMapProvider struct {
	UnavailableProvider
	SourceApplication string
}

func NewAMapProvider(source string) *AMapProvider {
	if source == "" {
		source = "journeyin"
	}
	return &AMapProvider{UnavailableProvider: NewUnavailableProvider(ProviderAMap), SourceApplication: source}
}
func (p *AMapProvider) NavigationURL(target NavTarget, mode TravelMode, platform Platform) (string, error) {
	if target.Name == "" {
		return "", fmt.Errorf("navigation target name is required")
	}
	if target.Location.CRS != CRSGCJ02 {
		return "", fmt.Errorf("amap navigation requires gcj02 target coordinates")
	}
	modeValue := "car"
	switch mode {
	case ModeWalking:
		modeValue = "walk"
	case ModeCycling:
		modeValue = "ride"
	case ModeTransit:
		modeValue = "bus"
	}
	query := url.Values{}
	query.Set("to", fmt.Sprintf("%.8f,%.8f,%s", target.Location.Lng, target.Location.Lat, target.Name))
	query.Set("mode", modeValue)
	query.Set("coordinate", "gaode")
	query.Set("callnative", "0")
	query.Set("src", p.SourceApplication)
	if platform == PlatformAndroid || platform == PlatformIOS {
		query.Del("to")
		query.Del("mode")
		query.Del("coordinate")
		query.Del("callnative")
		query.Set("sourceApplication", p.SourceApplication)
		query.Set("dlat", fmt.Sprintf("%.8f", target.Location.Lat))
		query.Set("dlon", fmt.Sprintf("%.8f", target.Location.Lng))
		query.Set("dname", target.Name)
		query.Set("dev", "0")
		tValue := "0"
		switch mode {
		case ModeTransit:
			tValue = "1"
		case ModeWalking:
			tValue = "2"
		case ModeCycling:
			tValue = "3"
		}
		query.Set("t", tValue)
		prefix := "amapuri://route/plan/?"
		if platform == PlatformIOS {
			prefix = "iosamap://path?"
		}
		return prefix + query.Encode(), nil
	}
	return "https://uri.amap.com/navigation?" + query.Encode(), nil
}
