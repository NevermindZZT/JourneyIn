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
	tiff.Write([]byte{'I', 'I', 0x2A, 0x00})            // II*
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

func TestParseTiffCaptureMetadata(t *testing.T) {
	var tiff bytes.Buffer
	bo := binary.LittleEndian
	tiff.Write([]byte{'I', 'I', 0x2A, 0x00})
	_ = binary.Write(&tiff, bo, uint32(8))

	const ifd0Offset = 8
	const ifd0Entries = 3
	const exifOffset = ifd0Offset + 2 + ifd0Entries*12 + 4
	const exifEntries = 6
	const payloadOffset = exifOffset + 2 + exifEntries*12 + 4
	makeBytes := append([]byte("SONY"), 0)
	modelBytes := append([]byte("ILCE-7M4"), 0)
	lensBytes := append([]byte("FE 24-70mm F2.8 GM II"), 0)
	makeOffset := payloadOffset
	modelOffset := makeOffset + len(makeBytes)
	lensOffset := modelOffset + len(modelBytes)
	exposureOffset := lensOffset + len(lensBytes)
	fNumberOffset := exposureOffset + 8
	focalOffset := fNumberOffset + 8
	biasOffset := focalOffset + 8

	writeEntry := func(tag, tagType uint16, count uint32, value uint32) {
		_ = binary.Write(&tiff, bo, tag)
		_ = binary.Write(&tiff, bo, tagType)
		_ = binary.Write(&tiff, bo, count)
		_ = binary.Write(&tiff, bo, value)
	}

	_ = binary.Write(&tiff, bo, uint16(ifd0Entries))
	writeEntry(0x010F, 2, uint32(len(makeBytes)), uint32(makeOffset))
	writeEntry(0x0110, 2, uint32(len(modelBytes)), uint32(modelOffset))
	writeEntry(0x8769, 4, 1, uint32(exifOffset))
	_ = binary.Write(&tiff, bo, uint32(0))

	_ = binary.Write(&tiff, bo, uint16(exifEntries))
	writeEntry(0x829A, 5, 1, uint32(exposureOffset))
	writeEntry(0x829D, 5, 1, uint32(fNumberOffset))
	writeEntry(0x8827, 3, 1, 100)
	writeEntry(0x9204, 10, 1, uint32(biasOffset))
	writeEntry(0x920A, 5, 1, uint32(focalOffset))
	writeEntry(0xA434, 2, uint32(len(lensBytes)), uint32(lensOffset))
	_ = binary.Write(&tiff, bo, uint32(0))

	tiff.Write(makeBytes)
	tiff.Write(modelBytes)
	tiff.Write(lensBytes)
	_ = binary.Write(&tiff, bo, uint32(1))
	_ = binary.Write(&tiff, bo, uint32(250))
	_ = binary.Write(&tiff, bo, uint32(28))
	_ = binary.Write(&tiff, bo, uint32(10))
	_ = binary.Write(&tiff, bo, uint32(24))
	_ = binary.Write(&tiff, bo, uint32(1))
	_ = binary.Write(&tiff, bo, int32(-1))
	_ = binary.Write(&tiff, bo, int32(3))

	meta := &PhotoMetadata{}
	parseTiffMetadata(tiff.Bytes(), meta)
	if meta.CameraMake != "SONY" || meta.CameraModel != "ILCE-7M4" || meta.LensModel != "FE 24-70mm F2.8 GM II" {
		t.Fatalf("unexpected equipment metadata: %+v", meta)
	}
	if meta.ExposureTimeS == nil || math.Abs(*meta.ExposureTimeS-0.004) > 0.000001 {
		t.Fatalf("unexpected exposure time: %+v", meta.ExposureTimeS)
	}
	if meta.FNumber == nil || math.Abs(*meta.FNumber-2.8) > 0.000001 {
		t.Fatalf("unexpected f number: %+v", meta.FNumber)
	}
	if meta.ISO == nil || *meta.ISO != 100 {
		t.Fatalf("unexpected ISO: %+v", meta.ISO)
	}
	if meta.FocalLengthMM == nil || *meta.FocalLengthMM != 24 {
		t.Fatalf("unexpected focal length: %+v", meta.FocalLengthMM)
	}
	if meta.ExposureBiasEV == nil || math.Abs(*meta.ExposureBiasEV+1.0/3.0) > 0.000001 {
		t.Fatalf("unexpected exposure bias: %+v", meta.ExposureBiasEV)
	}
}
