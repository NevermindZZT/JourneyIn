package photos

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestParseExifTimeWithOffsetProvenance(t *testing.T) {
	local, offsetKnown := parseExifTimeWithOffset("2026:10:13 12:00:00", "")
	if offsetKnown || !local.Equal(time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected local EXIF time: %v known=%v", local, offsetKnown)
	}
	instant, offsetKnown := parseExifTimeWithOffset("2026:10:13 12:00:00", "+08:00")
	if !offsetKnown || !instant.Equal(time.Date(2026, 10, 13, 4, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected offset EXIF time: %v known=%v", instant, offsetKnown)
	}
}

func TestParseTiffDateTimeOriginalOffsetTags(t *testing.T) {
	var tiff bytes.Buffer
	bo := binary.LittleEndian
	tiff.Write([]byte{'I', 'I', 0x2A, 0x00})
	_ = binary.Write(&tiff, bo, uint32(8))
	const ifd0Offset = 8
	const exifOffset = ifd0Offset + 2 + 12 + 4
	const exifEntryCount = 2
	const dateOffset = exifOffset + 2 + exifEntryCount*12 + 4
	dateBytes := append([]byte("2026:10:13 12:00:00"), 0)
	offsetBytes := append([]byte("+08:00"), 0)
	_ = binary.Write(&tiff, bo, uint16(1))
	_ = binary.Write(&tiff, bo, uint16(0x8769))
	_ = binary.Write(&tiff, bo, uint16(4))
	_ = binary.Write(&tiff, bo, uint32(1))
	_ = binary.Write(&tiff, bo, uint32(exifOffset))
	_ = binary.Write(&tiff, bo, uint32(0))
	_ = binary.Write(&tiff, bo, uint16(exifEntryCount))
	_ = binary.Write(&tiff, bo, uint16(0x9003))
	_ = binary.Write(&tiff, bo, uint16(2))
	_ = binary.Write(&tiff, bo, uint32(len(dateBytes)))
	_ = binary.Write(&tiff, bo, uint32(dateOffset))
	_ = binary.Write(&tiff, bo, uint16(0x9011))
	_ = binary.Write(&tiff, bo, uint16(2))
	_ = binary.Write(&tiff, bo, uint32(len(offsetBytes)))
	_ = binary.Write(&tiff, bo, uint32(dateOffset+len(dateBytes)))
	_ = binary.Write(&tiff, bo, uint32(0))
	tiff.Write(dateBytes)
	tiff.Write(offsetBytes)
	meta := &PhotoMetadata{}
	parseTiffMetadata(tiff.Bytes(), meta)
	want := time.Date(2026, 10, 13, 4, 0, 0, 0, time.UTC)
	if meta.TakenAtSource != "exif_original" || !meta.TakenAtOffsetKnown || !meta.TakenAt.Equal(want) {
		t.Fatalf("EXIF offset tags were not parsed: %+v", meta)
	}
}
