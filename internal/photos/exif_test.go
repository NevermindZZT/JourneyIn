package photos

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func TestExtractMetadataSyntheticJpeg(t *testing.T) {
	// 构建一个合成的 JPEG + Exif 数据流 (LittleEndian)
	var tiff bytes.Buffer
	// TIFF Header
	tiff.Write([]byte{'I', 'I', 0x2A, 0x00}) // II*
	binary.Write(&tiff, binary.LittleEndian, uint32(8)) // IFD0 offset = 8

	// IFD0: 2 个条目 (DateTime, GPS IFD Pointer)
	binary.Write(&tiff, binary.LittleEndian, uint16(2))

	// Entry 1: DateTime (0x0132, ASCII, count=20, offset)
	dateTimeOffset := uint32(8 + 2 + 2*12 + 4 + 2 + 4*12 + 4)
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0132))
	binary.Write(&tiff, binary.LittleEndian, uint16(2)) // ASCII
	binary.Write(&tiff, binary.LittleEndian, uint32(20))
	binary.Write(&tiff, binary.LittleEndian, dateTimeOffset)

	// Entry 2: GPS IFD Pointer (0x8825, LONG, count=1, value)
	gpsIfdOffset := uint32(8 + 2 + 2*12 + 4)
	binary.Write(&tiff, binary.LittleEndian, uint16(0x8825))
	binary.Write(&tiff, binary.LittleEndian, uint16(4)) // LONG
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, gpsIfdOffset)

	// Next IFD = 0
	binary.Write(&tiff, binary.LittleEndian, uint32(0))

	// --- GPS IFD ---
	binary.Write(&tiff, binary.LittleEndian, uint16(4)) // 4 条目: LatRef, Lat, LngRef, Lng

	// 1. LatRef (0x0001, ASCII, count=2, val='N', 0, 0, 0)
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0001))
	binary.Write(&tiff, binary.LittleEndian, uint16(2))
	binary.Write(&tiff, binary.LittleEndian, uint32(2))
	tiff.Write([]byte{'N', 0, 0, 0})

	// 2. Lat (0x0002, RATIONAL, count=3, offset)
	latDataOffset := dateTimeOffset + 20
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0002))
	binary.Write(&tiff, binary.LittleEndian, uint16(5))
	binary.Write(&tiff, binary.LittleEndian, uint32(3))
	binary.Write(&tiff, binary.LittleEndian, latDataOffset)

	// 3. LngRef (0x0003, ASCII, count=2, val='E', 0, 0, 0)
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0003))
	binary.Write(&tiff, binary.LittleEndian, uint16(2))
	binary.Write(&tiff, binary.LittleEndian, uint32(2))
	tiff.Write([]byte{'E', 0, 0, 0})

	// 4. Lng (0x0004, RATIONAL, count=3, offset)
	lngDataOffset := latDataOffset + 24
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0004))
	binary.Write(&tiff, binary.LittleEndian, uint16(5))
	binary.Write(&tiff, binary.LittleEndian, uint32(3))
	binary.Write(&tiff, binary.LittleEndian, lngDataOffset)

	// Next IFD = 0
	binary.Write(&tiff, binary.LittleEndian, uint32(0))

	// --- Payload data ---
	timeBytes := append([]byte("2024:05:01 14:30:00"), 0)
	tiff.Write(timeBytes)

	// Lat: 39/1, 54/1, 314/10 (39 deg 54 min 31.4 sec N = 39.908722)
	binary.Write(&tiff, binary.LittleEndian, uint32(39))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, uint32(54))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, uint32(314))
	binary.Write(&tiff, binary.LittleEndian, uint32(10))

	// Lng: 116/1, 23/1, 510/10 (116 deg 23 min 51.0 sec E = 116.3975)
	binary.Write(&tiff, binary.LittleEndian, uint32(116))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, uint32(23))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, uint32(510))
	binary.Write(&tiff, binary.LittleEndian, uint32(10))

	// 现在组装完整 JPEG: SOI + APP1 (Exif + tiff) + SOS
	var jpeg bytes.Buffer
	jpeg.Write([]byte{0xFF, 0xD8}) // SOI
	jpeg.Write([]byte{0xFF, 0xE1}) // APP1

	exifPrefix := []byte{'E', 'x', 'i', 'f', 0, 0}
	app1Payload := append(exifPrefix, tiff.Bytes()...)
	app1Len := uint16(len(app1Payload) + 2)
	binary.Write(&jpeg, binary.BigEndian, app1Len)
	jpeg.Write(app1Payload)

	jpeg.Write([]byte{0xFF, 0xDA}) // SOS

	meta, err := ExtractMetadata(bytes.NewReader(jpeg.Bytes()))
	if err != nil {
		t.Fatalf("ExtractMetadata failed: %v", err)
	}

	expectedTime := time.Date(2024, 5, 1, 14, 30, 0, 0, time.UTC)
	if !meta.TakenAt.Equal(expectedTime) {
		t.Fatalf("expected time %v, got %v", expectedTime, meta.TakenAt)
	}

	if !meta.HasGPS {
		t.Fatalf("expected HasGPS to be true")
	}

	expectedLat := 39.0 + 54.0/60.0 + 31.4/3600.0
	expectedLng := 116.0 + 23.0/60.0 + 51.0/3600.0

	if math.Abs(meta.Latitude-expectedLat) > 0.0001 {
		t.Fatalf("expected lat %f, got %f", expectedLat, meta.Latitude)
	}
	if math.Abs(meta.Longitude-expectedLng) > 0.0001 {
		t.Fatalf("expected lng %f, got %f", expectedLng, meta.Longitude)
	}
}
