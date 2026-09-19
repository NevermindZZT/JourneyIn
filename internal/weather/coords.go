package weather

import (
	"math"
)

// WGS84ToGCJ02 将 WGS-84 坐标转换为 GCJ-02 (火星坐标系)
func WGS84ToGCJ02(lat, lng float64) (gcjLat, gcjLng float64) {
	if outOfChina(lat, lng) {
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

// GCJ02ToWGS84 将 GCJ-02 坐标逆转为 WGS-84 坐标 (单次反向补偿可达毫米至米级精度)
func GCJ02ToWGS84(lat, lng float64) (wgsLat, wgsLng float64) {
	if outOfChina(lat, lng) {
		return lat, lng
	}
	gLat, gLng := WGS84ToGCJ02(lat, lng)
	dLat := gLat - lat
	dLng := gLng - lng
	return lat - dLat, lng - dLng
}

// GCJ02ToBD09LL 将 GCJ-02 坐标转换为 BD-09LL (百度坐标系)
func GCJ02ToBD09LL(lat, lng float64) (bdLat, bdLng float64) {
	const xPi = math.Pi * 3000.0 / 180.0
	z := math.Sqrt(lng*lng+lat*lat) + 0.00002*math.Sin(lat*xPi)
	theta := math.Atan2(lat, lng) + 0.000003*math.Cos(lng*xPi)
	return z*math.Sin(theta) + 0.006, z*math.Cos(theta) + 0.0065
}

// BD09LLToGCJ02 将百度 BD-09LL 坐标转换为 GCJ-02 坐标
func BD09LLToGCJ02(lat, lng float64) (gcjLat, gcjLng float64) {
	const xPi = math.Pi * 3000.0 / 180.0
	x := lng - 0.0065
	y := lat - 0.006
	z := math.Sqrt(x*x+y*y) - 0.00002*math.Sin(y*xPi)
	theta := math.Atan2(y, x) - 0.000003*math.Cos(x*xPi)
	return z * math.Sin(theta), z * math.Cos(theta)
}

// BD09LLToWGS84 将百度 BD-09LL 坐标转换为 WGS-84 坐标
func BD09LLToWGS84(lat, lng float64) (wgsLat, wgsLng float64) {
	gLat, gLng := BD09LLToGCJ02(lat, lng)
	return GCJ02ToWGS84(gLat, gLng)
}

// NormalizeToWGS84 将任意支持的 CRS 坐标点转换为 WGS-84 (Lat, Lng)
func NormalizeToWGS84(point GeoPoint) (lat, lng float64) {
	switch point.CRS {
	case CRSBD09LL:
		return BD09LLToWGS84(point.Lat, point.Lng)
	case CRSGCJ02:
		return GCJ02ToWGS84(point.Lat, point.Lng)
	case CRSWGS84, "":
		return point.Lat, point.Lng
	default:
		return point.Lat, point.Lng
	}
}

// NormalizeToGCJ02 将任意支持的 CRS 坐标点转换为 GCJ-02 (Lat, Lng)
func NormalizeToGCJ02(point GeoPoint) (lat, lng float64) {
	switch point.CRS {
	case CRSBD09LL:
		return BD09LLToGCJ02(point.Lat, point.Lng)
	case CRSWGS84:
		return WGS84ToGCJ02(point.Lat, point.Lng)
	case CRSGCJ02, "":
		return point.Lat, point.Lng
	default:
		return point.Lat, point.Lng
	}
}

func outOfChina(lat, lng float64) bool {
	return lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271
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
