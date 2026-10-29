package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

var (
	errNoExifSegment = errors.New("no EXIF APP1 segment found")
	errNotJPEG       = errors.New("EXIF preservation currently requires a JPEG input")
)

var exifSignature = []byte{'E', 'x', 'i', 'f', 0, 0}

// extractExifSegment returns the original APP1 segment so EXIF data, including
// vendor-specific tags, can be copied without rebuilding the TIFF structure.
func extractExifSegment(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input for EXIF preservation: %w", err)
	}
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return nil, errNotJPEG
	}

	for offset := 2; offset < len(data); {
		if data[offset] != 0xff {
			return nil, fmt.Errorf("invalid JPEG marker at offset %d", offset)
		}
		for offset < len(data) && data[offset] == 0xff {
			offset++
		}
		if offset >= len(data) {
			break
		}
		markerStart := offset - 1
		marker := data[offset]
		offset++
		if marker == 0xda || marker == 0xd9 {
			break
		} // SOS or EOI.
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		} // Standalone markers.
		if offset+2 > len(data) {
			return nil, errors.New("truncated JPEG segment length")
		}
		segmentLength := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		if segmentLength < 2 || offset+segmentLength > len(data) {
			return nil, errors.New("invalid JPEG segment length")
		}
		segmentEnd := offset + segmentLength
		payload := data[offset+2 : segmentEnd]
		if marker == 0xe1 && bytes.HasPrefix(payload, exifSignature) {
			return append([]byte(nil), data[markerStart:segmentEnd]...), nil
		}
		offset = segmentEnd
	}
	return nil, errNoExifSegment
}

func insertExifSegment(jpegData, segment []byte, width, height int) ([]byte, []error, error) {
	if len(jpegData) < 2 || jpegData[0] != 0xff || jpegData[1] != 0xd8 {
		return nil, nil, errors.New("encoded output is not a JPEG")
	}
	if len(segment) == 0 {
		return jpegData, nil, nil
	}
	if len(segment) < 12 || segment[0] != 0xff || segment[1] != 0xe1 {
		return nil, nil, errors.New("invalid EXIF APP1 segment")
	}
	warnings, err := updateExifImageProperties(segment, width, height)
	if err != nil {
		return nil, warnings, err
	}
	result := make([]byte, 0, len(jpegData)+len(segment))
	result = append(result, jpegData[:2]...)
	result = append(result, segment...)
	result = append(result, jpegData[2:]...)
	return result, warnings, nil
}

func updateExifImageProperties(segment []byte, width, height int) ([]error, error) {
	const tiffOffset = 10 // JPEG marker/length and "Exif\0\0" signature.
	if len(segment) < tiffOffset+8 {
		return nil, errors.New("truncated EXIF TIFF header")
	}
	tiff := segment[tiffOffset:]
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, errors.New("invalid EXIF byte order")
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return nil, errors.New("invalid EXIF TIFF signature")
	}
	ifd0Offset := uint64(order.Uint32(tiff[4:8]))
	ifd0, err := tiffIFD(tiff, ifd0Offset, order)
	if err != nil {
		return nil, err
	}
	var exifIFDOffset uint32
	var warnings []error
	for i := 0; i < ifd0.count; i++ {
		entry := tiff[ifd0.entries+i*12 : ifd0.entries+(i+1)*12]
		tag := order.Uint16(entry[:2])
		switch tag {
		case 0x0112: // Orientation
			if err := setTIFFScalar(entry, order, 1); err != nil {
				warnings = append(warnings, fmt.Errorf("reset EXIF orientation: %w", err))
			}
		case 0x8769: // Exif IFD pointer
			if exifIFDOffset, err = tiffScalarOffset(entry, order); err != nil {
				warnings = append(warnings, fmt.Errorf("read EXIF IFD pointer: %w", err))
			}
		case 0x0100:
			if err := setTIFFScalar(entry, order, uint32(width)); err != nil {
				warnings = append(warnings, fmt.Errorf("update EXIF image width: %w", err))
			}
		case 0x0101:
			if err := setTIFFScalar(entry, order, uint32(height)); err != nil {
				warnings = append(warnings, fmt.Errorf("update EXIF image height: %w", err))
			}
		}
	}
	if exifIFDOffset == 0 {
		return warnings, nil
	}
	exifIFD, err := tiffIFD(tiff, uint64(exifIFDOffset), order)
	if err != nil {
		return warnings, fmt.Errorf("read EXIF sub-IFD: %w", err)
	}
	for i := 0; i < exifIFD.count; i++ {
		entry := tiff[exifIFD.entries+i*12 : exifIFD.entries+(i+1)*12]
		switch order.Uint16(entry[:2]) {
		case 0xa002:
			if err := setTIFFScalar(entry, order, uint32(width)); err != nil {
				warnings = append(warnings, fmt.Errorf("update EXIF pixel width: %w", err))
			}
		case 0xa003:
			if err := setTIFFScalar(entry, order, uint32(height)); err != nil {
				warnings = append(warnings, fmt.Errorf("update EXIF pixel height: %w", err))
			}
		}
	}
	return warnings, nil
}

type tiffIFDInfo struct{ entries, count int }

func tiffIFD(tiff []byte, offset uint64, order binary.ByteOrder) (tiffIFDInfo, error) {
	if offset > uint64(len(tiff)) || offset+2 > uint64(len(tiff)) {
		return tiffIFDInfo{}, errors.New("IFD offset is out of bounds")
	}
	count := int(order.Uint16(tiff[offset : offset+2]))
	entries := int(offset) + 2
	end := uint64(entries) + uint64(count)*12 + 4 // Entries and next-IFD pointer.
	if end > uint64(len(tiff)) {
		return tiffIFDInfo{}, errors.New("IFD entries are truncated")
	}
	return tiffIFDInfo{entries: entries, count: count}, nil
}

func tiffScalarOffset(entry []byte, order binary.ByteOrder) (uint32, error) {
	if len(entry) < 12 || order.Uint16(entry[2:4]) != 4 || order.Uint32(entry[4:8]) != 1 {
		return 0, errors.New("expected one LONG value")
	}
	return order.Uint32(entry[8:12]), nil
}

func setTIFFScalar(entry []byte, order binary.ByteOrder, value uint32) error {
	if len(entry) < 12 || order.Uint32(entry[4:8]) != 1 {
		return errors.New("expected a single-value TIFF tag")
	}
	switch order.Uint16(entry[2:4]) {
	case 3: // SHORT
		if value > 0xffff {
			return fmt.Errorf("value %d does not fit in SHORT", value)
		}
		order.PutUint16(entry[8:10], uint16(value))
	case 4: // LONG
		order.PutUint32(entry[8:12], value)
	default:
		return fmt.Errorf("unsupported TIFF scalar type %d", order.Uint16(entry[2:4]))
	}
	return nil
}
