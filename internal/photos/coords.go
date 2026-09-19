package photos

import (
	"math"
)

// WGS84ToGCJ02 将 WGS-84 坐标转换为 GCJ-02 (火星坐标系)
func WGS84ToGCJ02(lat, lng float64) (gcjLat, gcjLng float64) {
	if lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271 {
		return lat, lng
	}
	const a = 6378245.0
	const ee = 0.00669342162296594323
	dLat := transformLatitude(lng-105.0, lat-35.0)
	dLng := transformLongitude(lng-105.0, lat-35.0)
	radLat := lat / 180.0 * math.Pi
	magic := math.Sin(radLat)
	magic = 1 - ee*magic*magic
	sqrtMagic := math.Sqrt(magic)
	dLat = dLat * 180.0 / ((a * (1 - ee)) / (magic * sqrtMagic) * math.Pi)
	dLng = dLng * 180.0 / (a / sqrtMagic * math.Cos(radLat) * math.Pi)
	return lat + dLat, lng + dLng
}

// GCJ02ToBD09LL 将 GCJ-02 坐标转换为 BD-09LL (百度坐标系)
func GCJ02ToBD09LL(lat, lng float64) (bdLat, bdLng float64) {
	const xPi = math.Pi * 3000.0 / 180.0
	z := math.Sqrt(lng*lng+lat*lat) + 0.00002*math.Sin(lat*xPi)
	theta := math.Atan2(lat, lng) + 0.000003*math.Cos(lng*xPi)
	return z*math.Sin(theta) + 0.006, z*math.Cos(theta) + 0.0065
}

// WGS84ToBD09LL 将 WGS-84 坐标直接转换为 BD-09LL
func WGS84ToBD09LL(lat, lng float64) (bdLat, bdLng float64) {
	gcjLat, gcjLng := WGS84ToGCJ02(lat, lng)
	return GCJ02ToBD09LL(gcjLat, gcjLng)
}

func transformLatitude(x, y float64) float64 {
	ret := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(y*math.Pi) + 40.0*math.Sin(y/3.0*math.Pi)) * 2.0 / 3.0
	ret += (160.0*math.Sin(y/12.0*math.Pi) + 320.0*math.Sin(y*math.Pi/30.0)) * 2.0 / 3.0
	return ret
}

func transformLongitude(x, y float64) float64 {
	ret := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(x*math.Pi) + 40.0*math.Sin(x/3.0*math.Pi)) * 2.0 / 3.0
	ret += (150.0*math.Sin(x/12.0*math.Pi) + 300.0*math.Sin(x/30.0*math.Pi)) * 2.0 / 3.0
	return ret
}
