package main

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

type ExifData struct {
	Make, Model, LensMake, LensModel string
	ExposureTime, FNumber, ISO       string
	FocalLength35mm, FocalLength     string
	DateTimeOriginal                 string
}

type exifTag struct {
	name string
	id   uint16
	path string
	set  func(*ExifData, string) error
}

const (
	ifdPath     = "IFD"
	exifIFDPath = "IFD/Exif"
)

var exifTags = []exifTag{
	{name: "Make", id: 0x010f, path: ifdPath, set: setMake},
	{name: "Model", id: 0x0110, path: ifdPath, set: setModel},
	{name: "LensMake", id: 0xa433, path: exifIFDPath, set: setLensMake},
	{name: "LensModel", id: 0xa434, path: exifIFDPath, set: setLensModel},
	{name: "ExposureTime", id: 0x829a, path: exifIFDPath, set: setExposureTime},
	{name: "FNumber", id: 0x829d, path: exifIFDPath, set: setFNumber},
	{name: "PhotographicSensitivity", id: 0x8827, path: exifIFDPath, set: setISO},
	{name: "FocalLengthIn35mmFilm", id: 0xa405, path: exifIFDPath, set: setFocalLength35mm},
	{name: "FocalLength", id: 0x920a, path: exifIFDPath, set: setFocalLength},
	{name: "DateTimeOriginal", id: 0x9003, path: exifIFDPath, set: setDateTimeOriginal},
}

func parseRational(value string) (float64, error) {
	parts := strings.Split(value, "/")
	if len(parts) == 1 {
		return strconv.ParseFloat(parts[0], 64)
	}
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid rational %q", value)
	}
	numerator, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	denominator, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, err
	}
	if denominator == 0 {
		return 0, fmt.Errorf("invalid rational %q: zero denominator", value)
	}
	return numerator / denominator, nil
}

func getExif(path string) (*ExifData, []error, error) {
	data := &ExifData{}
	raw, err := exif.SearchFileAndExtractExif(path)
	if err != nil {
		return data, nil, fmt.Errorf("read EXIF: %w", err)
	}
	mapping, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return data, nil, fmt.Errorf("create EXIF mapping: %w", err)
	}
	_, index, err := exif.Collect(mapping, exif.NewTagIndex(), raw)
	if err != nil {
		return data, nil, fmt.Errorf("parse EXIF: %w", err)
	}
	type ifdResult struct {
		ifd *exif.Ifd
		err error
	}
	ifds := map[string]ifdResult{}
	var warnings []error
	for _, tag := range exifTags {
		result, exists := ifds[tag.path]
		if !exists {
			result.ifd, result.err = exif.FindIfdFromRootIfd(index.RootIfd, tag.path)
			ifds[tag.path] = result
		}
		if result.err != nil {
			continue
		}
		items, err := result.ifd.FindTagWithId(tag.id)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("find EXIF tag %s: %w", tag.name, err))
			continue
		}
		if len(items) == 0 {
			continue
		}
		value, err := items[0].FormatFirst()
		if err != nil {
			warnings = append(warnings, fmt.Errorf("format EXIF tag %s: %w", tag.name, err))
			continue
		}
		if err := tag.set(data, value); err != nil {
			warnings = append(warnings, fmt.Errorf("decode EXIF tag %s: %w", tag.name, err))
		}
	}
	return data, warnings, nil
}

func setMake(d *ExifData, v string) error      { d.Make = v; return nil }
func setModel(d *ExifData, v string) error     { d.Model = v; return nil }
func setLensMake(d *ExifData, v string) error  { d.LensMake = v; return nil }
func setLensModel(d *ExifData, v string) error { d.LensModel = v; return nil }
func setExposureTime(d *ExifData, value string) error {
	formatted, err := formatExposureTime(value)
	if err != nil {
		return err
	}
	d.ExposureTime = formatted
	return nil
}
func setISO(d *ExifData, v string) error             { d.ISO = v; return nil }
func setFocalLength35mm(d *ExifData, v string) error { d.FocalLength35mm = v; return nil }
func setFocalLength(d *ExifData, v string) error     { d.FocalLength = v; return nil }

func setFNumber(d *ExifData, value string) error {
	n, err := parseRational(value)
	if err != nil {
		return err
	}
	d.FNumber = strconv.FormatFloat(n, 'f', 1, 64)
	return nil
}

func setDateTimeOriginal(d *ExifData, value string) error {
	t, err := time.Parse("2006:01:02 15:04:05", value)
	if err != nil {
		return err
	}
	d.DateTimeOriginal = t.Format("2006/01/02 15:04")
	return nil
}

func formatExposureTime(value string) (string, error) {
	seconds, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok || seconds.Sign() <= 0 {
		return "", fmt.Errorf("invalid exposure time %q", value)
	}
	if seconds.Cmp(big.NewRat(1, 1)) >= 0 {
		if seconds.IsInt() {
			return seconds.Num().String(), nil
		}
		decimal := seconds.FloatString(3)
		decimal = strings.TrimRight(strings.TrimRight(decimal, "0"), ".")
		return decimal, nil
	}
	reciprocal := new(big.Rat).Inv(seconds)
	if reciprocal.IsInt() {
		return "1/" + reciprocal.Num().String(), nil
	}
	return seconds.RatString(), nil
}
