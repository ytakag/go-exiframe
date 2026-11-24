package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

func hasExifOverrides(c Config) bool {
	return c.exifMake != "" || c.exifModel != "" || c.exifLensMake != "" || c.exifLensModel != "" || c.exifFocalLength != "" || c.exifFocalLength35mm != "" || c.exifFNumber != "" || c.exifExposureTime != "" || c.exifISO != "" || c.exifDateTime != ""
}

func applyExifOverrides(d *ExifData, c Config) {
	if c.exifMake != "" {
		d.Make = c.exifMake
	}
	if c.exifModel != "" {
		d.Model = c.exifModel
	}
	if c.exifLensMake != "" {
		d.LensMake = c.exifLensMake
	}
	if c.exifLensModel != "" {
		d.LensModel = c.exifLensModel
	}
	if c.exifFocalLength != "" {
		d.FocalLength, _ = formatFocalLength(c.exifFocalLength)
	}
	if c.exifFocalLength35mm != "" {
		d.FocalLength35mm = c.exifFocalLength35mm
	}
	if c.exifFNumber != "" {
		n, _ := parseRational(c.exifFNumber)
		d.FNumber = strconv.FormatFloat(n, 'f', 1, 64)
	}
	if c.exifExposureTime != "" {
		d.ExposureTime, _ = formatExposureTime(c.exifExposureTime)
	}
	if c.exifISO != "" {
		d.ISO = c.exifISO
	}
	if c.exifDateTime != "" {
		t, _ := time.Parse("2006:01:02 15:04:05", c.exifDateTime)
		d.DateTimeOriginal = t.Format("2006/01/02 15:04")
	}
}

func rationalValue(s string) (exifcommon.Rational, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return exifcommon.Rational{}, fmt.Errorf("invalid positive rational %q", s)
	}
	const scale = int64(1000000)
	n := new(big.Int).Mul(r.Num(), big.NewInt(scale))
	n.Quo(n, r.Denom())
	if !n.IsUint64() || n.Uint64() > 0xffffffff {
		return exifcommon.Rational{}, fmt.Errorf("rational %q is too large", s)
	}
	return exifcommon.Rational{Numerator: uint32(n.Uint64()), Denominator: uint32(scale)}, nil
}

func buildExifSegment(original []byte, c Config) ([]byte, error) {
	var root *exif.IfdBuilder
	var mapping *exifcommon.IfdMapping
	if len(original) > 0 {
		if len(original) < 12 || original[0] != 0xff || original[1] != 0xe1 {
			return nil, fmt.Errorf("invalid source EXIF segment")
		}
		// Collect expects the TIFF header (II/MM), without the JPEG marker,
		// segment length, or the six-byte "Exif\0\0" prefix.
		raw := original[10:]
		im, err := exifcommon.NewIfdMappingWithStandard()
		if err != nil {
			return nil, err
		}
		_, ti, err := exif.Collect(im, exif.NewTagIndex(), raw)
		if err != nil {
			return nil, fmt.Errorf("parse source EXIF for update: %w", err)
		}
		root = exif.NewIfdBuilderFromExistingChain(ti.RootIfd)
		mapping = im
	} else {
		mapping = exifcommon.NewIfdMapping()
		if err := exifcommon.LoadStandardIfds(mapping); err != nil {
			return nil, err
		}
		index := exif.NewTagIndex()
		root = exif.NewIfdBuilder(mapping, index, exifcommon.IfdStandardIfdIdentity, binary.LittleEndian)
		exifIFD := exif.NewIfdBuilder(mapping, index, exifcommon.IfdExifStandardIfdIdentity, binary.LittleEndian)
		if err := root.AddChildIb(exifIFD); err != nil {
			return nil, err
		}
	}
	child, err := root.ChildWithTagId(0x8769)
	if err != nil {
		child = exif.NewIfdBuilder(mapping, exif.NewTagIndex(), exifcommon.IfdExifStandardIfdIdentity, binary.LittleEndian)
		if err := root.AddChildIb(child); err != nil {
			return nil, fmt.Errorf("create EXIF sub-IFD: %w", err)
		}
	}
	setString := func(ifd *exif.IfdBuilder, tag uint16, value string) error {
		if value == "" {
			return nil
		}
		return ifd.SetStandard(tag, value)
	}
	for _, item := range []struct {
		ifd   *exif.IfdBuilder
		tag   uint16
		value string
	}{
		{root, 0x010f, c.exifMake}, {root, 0x0110, c.exifModel}, {child, 0xa433, c.exifLensMake}, {child, 0xa434, c.exifLensModel},
	} {
		if err := setString(item.ifd, item.tag, item.value); err != nil {
			return nil, fmt.Errorf("set EXIF tag 0x%04x: %w", item.tag, err)
		}
	}
	for _, item := range []struct {
		tag   uint16
		value string
	}{{0x920a, c.exifFocalLength}, {0x829d, c.exifFNumber}, {0x829a, c.exifExposureTime}} {
		if item.value == "" {
			continue
		}
		v, err := rationalValue(item.value)
		if err != nil {
			return nil, err
		}
		if err := child.SetStandard(item.tag, []exifcommon.Rational{v}); err != nil {
			return nil, err
		}
	}
	if c.exifFocalLength35mm != "" {
		var n uint16
		if _, err := fmt.Sscan(c.exifFocalLength35mm, &n); err != nil {
			return nil, err
		}
		if err := child.SetStandard(0xa405, []uint16{n}); err != nil {
			return nil, err
		}
	}
	if c.exifISO != "" {
		var n uint16
		if _, err := fmt.Sscan(c.exifISO, &n); err != nil {
			return nil, err
		}
		if err := child.SetStandard(0x8827, []uint16{n}); err != nil {
			return nil, err
		}
	}
	if c.exifDateTime != "" {
		t, _ := time.Parse("2006:01:02 15:04:05", c.exifDateTime)
		if err := child.SetStandard(0x9003, t.Format("2006:01:02 15:04:05")); err != nil {
			return nil, err
		}
	}
	raw, err := exif.NewIfdByteEncoder().EncodeToExif(root)
	if err != nil {
		return nil, fmt.Errorf("encode EXIF: %w", err)
	}
	if len(raw)+len(exifSignature)+2 > 65535 {
		return nil, fmt.Errorf("EXIF data exceeds JPEG APP1 size limit")
	}
	var segment bytes.Buffer
	segment.Write([]byte{0xff, 0xe1})
	_ = binary.Write(&segment, binary.BigEndian, uint16(len(raw)+len(exifSignature)+2))
	segment.Write(exifSignature)
	segment.Write(raw)
	return segment.Bytes(), nil
}
