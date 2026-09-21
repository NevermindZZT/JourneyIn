package photos

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"time"
)

// PhotoMetadata 包含从照片中提取的拍摄时间、GPS 坐标与非敏感拍摄参数。
type PhotoMetadata struct {
	TakenAt   time.Time
	HasGPS    bool
	Latitude  float64 // WGS-84 原始纬度
	Longitude float64 // WGS-84 原始经度

	CameraMake     string
	CameraModel    string
	LensModel      string
	ExposureTimeS  *float64
	FNumber        *float64
	ISO            *int
	FocalLengthMM  *float64
	ExposureBiasEV *float64
}

var errNoExif = errors.New("no exif metadata found")

// ExtractMetadata 从 reader 中读取前段字节流并提取拍摄时间与 GPS 信息
func ExtractMetadata(r io.Reader) (*PhotoMetadata, error) {
	const maxHeaderSize = 128 * 1024
	header := make([]byte, maxHeaderSize)
	n, err := io.ReadFull(r, header)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	buf := header[:n]

	meta := &PhotoMetadata{}

	// 1. 尝试作为 JPEG 查找 APP1 (0xFFE1) Exif
	if len(buf) > 4 && buf[0] == 0xFF && buf[1] == 0xD8 {
		tiffData := findJpegExifTiff(buf)
		if tiffData != nil {
			parseTiffMetadata(tiffData, meta)
			return meta, nil
		}
	}

	// 2. 尝试直接作为 TIFF / DNG 头部检查 (II* 或 MM*)
	if len(buf) > 8 && (bytes.HasPrefix(buf, []byte{0x49, 0x49, 0x2A, 0x00}) || bytes.HasPrefix(buf, []byte{0x4D, 0x4D, 0x00, 0x2A})) {
		parseTiffMetadata(buf, meta)
		return meta, nil
	}

	// 3. 尝试在 PNG 中查找 eXIf chunk
	if len(buf) > 8 && bytes.HasPrefix(buf, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		tiffData := findPngExifTiff(buf)
		if tiffData != nil {
			parseTiffMetadata(tiffData, meta)
			return meta, nil
		}
	}

	return meta, errNoExif
}

func findJpegExifTiff(buf []byte) []byte {
	offset := 2
	for offset+4 <= len(buf) {
		if buf[offset] != 0xFF {
			return nil
		}
		marker := buf[offset+1]
		offset += 2

		if marker == 0xDA || marker == 0xD9 {
			return nil
		}

		if offset+2 > len(buf) {
			return nil
		}
		length := int(binary.BigEndian.Uint16(buf[offset : offset+2]))
		if length < 2 || offset+length > len(buf) {
			return nil
		}

		segmentData := buf[offset+2 : offset+length]
		if marker == 0xE1 { // APP1
			if len(segmentData) >= 6 && bytes.Equal(segmentData[:6], []byte{0x45, 0x78, 0x69, 0x66, 0x00, 0x00}) {
				return segmentData[6:]
			}
		}
		offset += length
	}
	return nil
}

func findPngExifTiff(buf []byte) []byte {
	offset := 8
	for offset+8 <= len(buf) {
		chunkLen := int(binary.BigEndian.Uint32(buf[offset : offset+4]))
		chunkType := string(buf[offset+4 : offset+8])
		offset += 8
		if chunkLen < 0 || offset+chunkLen+4 > len(buf) {
			return nil
		}
		if chunkType == "eXIf" {
			return buf[offset : offset+chunkLen]
		}
		offset += chunkLen + 4
	}
	return nil
}

func parseTiffMetadata(tiff []byte, meta *PhotoMetadata) {
	if len(tiff) < 8 {
		return
	}
	var bo binary.ByteOrder
	if tiff[0] == 0x49 && tiff[1] == 0x49 {
		bo = binary.LittleEndian
	} else if tiff[0] == 0x4D && tiff[1] == 0x4D {
		bo = binary.BigEndian
	} else {
		return
	}

	if bo.Uint16(tiff[2:4]) != 42 {
		return
	}

	ifd0Offset := int(bo.Uint32(tiff[4:8]))
	if ifd0Offset < 8 || ifd0Offset >= len(tiff) {
		return
	}

	var exifOffset int
	var gpsOffset int

	// 解析 IFD0
	parseIFD(tiff, ifd0Offset, bo, func(tag uint16, tagType uint16, count uint32, valBuf []byte) {
		switch tag {
		case 0x010F: // Make
			meta.CameraMake = strings.TrimSpace(readAscii(tiff, valBuf, count))
		case 0x0110: // Model
			meta.CameraModel = strings.TrimSpace(readAscii(tiff, valBuf, count))
		case 0x0132: // DateTime
			if meta.TakenAt.IsZero() {
				meta.TakenAt = parseExifTime(readAscii(tiff, valBuf, count))
			}
		case 0x8769: // Exif IFD Pointer
			exifOffset = int(bo.Uint32(valBuf[:4]))
		case 0x8825: // GPS IFD Pointer
			gpsOffset = int(bo.Uint32(valBuf[:4]))
		}
	})

	// 解析 Exif 子 IFD (获取 DateTimeOriginal)
	if exifOffset > 0 && exifOffset < len(tiff) {
		parseIFD(tiff, exifOffset, bo, func(tag uint16, tagType uint16, count uint32, valBuf []byte) {
			switch tag {
			case 0x829A: // ExposureTime
				meta.ExposureTimeS = readRational(tiff, valBuf, bo)
			case 0x829D: // FNumber
				meta.FNumber = readRational(tiff, valBuf, bo)
			case 0x8827: // PhotographicSensitivity / ISO
				meta.ISO = readUnsignedShort(valBuf, bo)
			case 0x9003: // DateTimeOriginal
				t := parseExifTime(readAscii(tiff, valBuf, count))
				if !t.IsZero() {
					meta.TakenAt = t
				}
			case 0x9004: // DateTimeDigitized
				if meta.TakenAt.IsZero() {
					meta.TakenAt = parseExifTime(readAscii(tiff, valBuf, count))
				}
			case 0x9204: // ExposureBiasValue
				meta.ExposureBiasEV = readSignedRational(tiff, valBuf, bo)
			case 0x920A: // FocalLength
				meta.FocalLengthMM = readRational(tiff, valBuf, bo)
			case 0xA434: // LensModel
				meta.LensModel = strings.TrimSpace(readAscii(tiff, valBuf, count))
			}
		})
	}

	// 解析 GPS 子 IFD
	if gpsOffset > 0 && gpsOffset < len(tiff) {
		var latRef, lngRef string
		var latDegs, lngDegs []float64

		parseIFD(tiff, gpsOffset, bo, func(tag uint16, tagType uint16, count uint32, valBuf []byte) {
			switch tag {
			case 0x0001: // GPSLatitudeRef
				latRef = strings.TrimSpace(readAscii(tiff, valBuf, count))
			case 0x0002: // GPSLatitude
				latDegs = readRationals(tiff, valBuf, count, bo)
			case 0x0003: // GPSLongitudeRef
				lngRef = strings.TrimSpace(readAscii(tiff, valBuf, count))
			case 0x0004: // GPSLongitude
				lngDegs = readRationals(tiff, valBuf, count, bo)
			}
		})

		if len(latDegs) == 3 && len(lngDegs) == 3 {
			lat := latDegs[0] + latDegs[1]/60.0 + latDegs[2]/3600.0
			lng := lngDegs[0] + lngDegs[1]/60.0 + lngDegs[2]/3600.0

			if strings.EqualFold(latRef, "S") {
				lat = -lat
			}
			if strings.EqualFold(lngRef, "W") {
				lng = -lng
			}

			if (lat != 0 || lng != 0) && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 {
				meta.Latitude = lat
				meta.Longitude = lng
				meta.HasGPS = true
			}
		}
	}
}

func parseIFD(tiff []byte, offset int, bo binary.ByteOrder, fn func(tag, tagType uint16, count uint32, valBuf []byte)) {
	if offset+2 > len(tiff) {
		return
	}
	numEntries := int(bo.Uint16(tiff[offset : offset+2]))
	curr := offset + 2

	for i := 0; i < numEntries; i++ {
		if curr+12 > len(tiff) {
			break
		}
		entry := tiff[curr : curr+12]
		curr += 12

		tag := bo.Uint16(entry[0:2])
		tagType := bo.Uint16(entry[2:4])
		count := bo.Uint32(entry[4:8])
		valBuf := entry[8:12]

		fn(tag, tagType, count, valBuf)
	}
}

func readAscii(tiff []byte, valBuf []byte, count uint32) string {
	if count <= 4 {
		return trimNull(string(valBuf[:count]))
	}
	var bo binary.ByteOrder = binary.LittleEndian
	if tiff[0] == 0x4D {
		bo = binary.BigEndian
	}
	offset := int(bo.Uint32(valBuf[:4]))
	if offset < 0 || offset+int(count) > len(tiff) {
		return ""
	}
	return trimNull(string(tiff[offset : offset+int(count)]))
}

func trimNull(s string) string {
	return strings.TrimRight(s, string([]byte{0}))
}

func readRationals(tiff []byte, valBuf []byte, count uint32, bo binary.ByteOrder) []float64 {
	offset := int(bo.Uint32(valBuf[:4]))
	byteLen := int(count) * 8
	if offset < 0 || offset+byteLen > len(tiff) {
		return nil
	}
	res := make([]float64, 0, count)
	for i := 0; i < int(count); i++ {
		entryOffset := offset + i*8
		num := bo.Uint32(tiff[entryOffset : entryOffset+4])
		den := bo.Uint32(tiff[entryOffset+4 : entryOffset+8])
		if den == 0 {
			res = append(res, 0)
		} else {
			res = append(res, float64(num)/float64(den))
		}
	}
	return res
}

func readUnsignedShort(valBuf []byte, bo binary.ByteOrder) *int {
	if len(valBuf) < 2 {
		return nil
	}
	value := int(bo.Uint16(valBuf[:2]))
	if value <= 0 {
		return nil
	}
	return &value
}

func readRational(tiff []byte, valBuf []byte, bo binary.ByteOrder) *float64 {
	if len(valBuf) < 4 {
		return nil
	}
	offset := int(bo.Uint32(valBuf[:4]))
	if offset < 0 || offset+8 > len(tiff) {
		return nil
	}
	numerator := bo.Uint32(tiff[offset : offset+4])
	denominator := bo.Uint32(tiff[offset+4 : offset+8])
	if denominator == 0 {
		return nil
	}
	value := float64(numerator) / float64(denominator)
	return &value
}

func readSignedRational(tiff []byte, valBuf []byte, bo binary.ByteOrder) *float64 {
	if len(valBuf) < 4 {
		return nil
	}
	offset := int(bo.Uint32(valBuf[:4]))
	if offset < 0 || offset+8 > len(tiff) {
		return nil
	}
	numerator := int32(bo.Uint32(tiff[offset : offset+4]))
	denominator := int32(bo.Uint32(tiff[offset+4 : offset+8]))
	if denominator == 0 {
		return nil
	}
	value := float64(numerator) / float64(denominator)
	return &value
}

func parseExifTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006:01:02 15:04:05",
		"2006-01-02 15:04:05",
		"2006:01:02T15:04:05",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
