package weather

import (
	"math"
	"testing"
)

func TestNormalizeCoords(t *testing.T) {
	// 杭州西湖 (约 30.24, 120.15)
	wgsLat, wgsLng := 30.2436, 120.1512
	gcjLat, gcjLng := WGS84ToGCJ02(wgsLat, wgsLng)
	bdLat, bdLng := GCJ02ToBD09LL(gcjLat, gcjLng)

	// 测试 BD09LL -> WGS84 往返还原精度
	convertedLat, convertedLng := BD09LLToWGS84(bdLat, bdLng)
	if math.Abs(convertedLat-wgsLat) > 0.0001 || math.Abs(convertedLng-wgsLng) > 0.0001 {
		t.Errorf("BD09LLToWGS84 mismatch: got (%f, %f), expected (%f, %f)", convertedLat, convertedLng, wgsLat, wgsLng)
	}

	// 测试 NormalizeToWGS84
	normLat, normLng := NormalizeToWGS84(GeoPoint{Lat: bdLat, Lng: bdLng, CRS: CRSBD09LL})
	if math.Abs(normLat-wgsLat) > 0.0001 || math.Abs(normLng-wgsLng) > 0.0001 {
		t.Errorf("NormalizeToWGS84 mismatch: got (%f, %f), expected (%f, %f)", normLat, normLng, wgsLat, wgsLng)
	}

	// 测试 NormalizeToGCJ02
	normGCJLat, normGCJLng := NormalizeToGCJ02(GeoPoint{Lat: wgsLat, Lng: wgsLng, CRS: CRSWGS84})
	if math.Abs(normGCJLat-gcjLat) > 0.00001 || math.Abs(normGCJLng-gcjLng) > 0.00001 {
		t.Errorf("NormalizeToGCJ02 mismatch: got (%f, %f), expected (%f, %f)", normGCJLat, normGCJLng, gcjLat, gcjLng)
	}
}