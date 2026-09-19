package photos

import (
	"math"
	"testing"
)

func TestCoordsConversion(t *testing.T) {
	// 天安门原始 WGS84 坐标
	wgsLat, wgsLng := 39.908722, 116.397499

	gcjLat, gcjLng := WGS84ToGCJ02(wgsLat, wgsLng)
	if math.Abs(gcjLat-39.9099) > 0.01 || math.Abs(gcjLng-116.4038) > 0.01 {
		t.Fatalf("unexpected GCJ02: lat=%f, lng=%f", gcjLat, gcjLng)
	}

	bdLat, bdLng := WGS84ToBD09LL(wgsLat, wgsLng)
	if math.Abs(bdLat-39.9161) > 0.01 || math.Abs(bdLng-116.4103) > 0.01 {
		t.Fatalf("unexpected BD09LL: lat=%f, lng=%f", bdLat, bdLng)
	}
}
