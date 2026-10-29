package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/math/fixed"
)

func loadFaces(metrics layoutMetrics) (font.Face, font.Face, error) {
	boldFont, err := truetype.Parse(gomonobold.TTF)
	if err != nil {
		return nil, nil, fmt.Errorf("parse bold font: %w", err)
	}
	regularFont, err := truetype.Parse(gomono.TTF)
	if err != nil {
		return nil, nil, fmt.Errorf("parse regular font: %w", err)
	}
	return truetype.NewFace(boldFont, &truetype.Options{Size: metrics.largeFontSize}), truetype.NewFace(regularFont, &truetype.Options{Size: metrics.smallFontSize}), nil
}

func renderImage(cfg Config, inputPath, outputPath string, meta *ExifData, exifSegment []byte) error {
	source, err := imaging.Open(inputPath, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}
	metrics := calculateLayoutMetrics(source.Bounds().Dx(), source.Bounds().Dy(), cfg)
	bold, regular, err := loadFaces(metrics)
	if err != nil {
		return err
	}
	defer bold.Close()
	defer regular.Close()
	left, right := buildLabelText(meta, cfg, bold, regular)
	layout, err := layoutLabel(source.Bounds().Dx(), source.Bounds().Dy(), metrics, left, right)
	if err != nil {
		return err
	}
	canvas := image.NewRGBA(layout.CanvasBounds)
	frameColor, textColor := image.White, image.Black
	if cfg.black {
		frameColor, textColor = image.Black, image.White
	}
	if cfg.overlayPosition != overlayNone {
		frameColor, textColor = image.Black, image.White
		if cfg.black {
			frameColor, textColor = image.White, image.Black
		}
	}
	draw.Draw(canvas, canvas.Bounds(), frameColor, image.Point{}, draw.Src)
	draw.Draw(canvas, layout.PhotoBounds, source, source.Bounds().Min, draw.Src)
	if !layout.OverlayBounds.Empty() {
		backdrop := color.RGBA{R: 0, G: 0, B: 0, A: 175}
		if cfg.overlayPosition != overlayFullWidth {
			backdrop.A = 150
		}
		if cfg.black {
			backdrop = color.RGBA{R: 255, G: 255, B: 255, A: 210}
			if cfg.overlayPosition != overlayFullWidth {
				backdrop.A = 180
			}
		}
		draw.Draw(canvas, layout.OverlayBounds, image.NewUniform(backdrop), image.Point{}, draw.Over)
	}
	for _, line := range layout.Text {
		d := &font.Drawer{Dst: canvas, Src: textColor, Face: line.face, Dot: fixed.P(line.x, line.baseline)}
		d.DrawString(line.text)
	}
	return writeJPEG(outputPath, canvas, cfg.quality, exifSegment)
}

func writeJPEG(path string, img image.Image, quality int, exifSegment []byte) (result error) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: quality}); err != nil {
		return fmt.Errorf("encode JPEG: %w", err)
	}
	jpegData := encoded.Bytes()
	if len(exifSegment) > 0 {
		var warnings []error
		var err error
		jpegData, warnings, err = insertExifSegment(jpegData, exifSegment, img.Bounds().Dx(), img.Bounds().Dy())
		for _, warning := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %v\n", warning)
		}
		if err != nil {
			return fmt.Errorf("preserve EXIF: %w", err)
		}
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".go-exiframe-*.jpg")
	if err != nil {
		return fmt.Errorf("create temporary output in %q: %w", dir, err)
	}
	tempPath := temp.Name()
	defer func() {
		if result != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(jpegData); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write JPEG: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync JPEG: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close JPEG: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace output %q: %w", path, err)
	}
	return nil
}
