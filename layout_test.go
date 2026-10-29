package main

import (
	"testing"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
)

func testFaces(t *testing.T, metrics layoutMetrics) (font.Face, font.Face) {
	t.Helper()
	boldFont, err := truetype.Parse(gomonobold.TTF)
	if err != nil {
		t.Fatal(err)
	}
	regularFont, err := truetype.Parse(gomono.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return truetype.NewFace(boldFont, &truetype.Options{Size: metrics.largeFontSize}),
		truetype.NewFace(regularFont, &truetype.Options{Size: metrics.smallFontSize})
}

func TestLayoutLabelOuterGaps(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		frameRatio    float64
		noFrame       bool
	}{
		{name: "landscape with frame", width: 1800, height: 1200, frameRatio: 0.03},
		{name: "portrait with wide frame", width: 900, height: 1500, frameRatio: 0.12},
		{name: "no frame", width: 1000, height: 1600, frameRatio: 0.2, noFrame: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{frameRatio: tt.frameRatio, noFrame: tt.noFrame}
			metrics := calculateLayoutMetrics(tt.width, tt.height, cfg)
			bold, regular := testFaces(t, metrics)
			defer bold.Close()
			defer regular.Close()
			meta := &ExifData{
				Make: "Camera Maker", Model: "Camera Model",
				LensMake: "Lens Maker", LensModel: "Lens Model",
				FocalLength35mm: "50", FNumber: "2.8", ExposureTime: "1/125", ISO: "400",
				DateTimeOriginal: "2026/10/29 12:34",
			}
			left, right := buildLabelText(meta, cfg, bold, regular)
			layout := layoutLabel(tt.width, tt.height, metrics, left, right)
			if len(layout.Text) == 0 {
				t.Fatal("expected label text")
			}

			firstTop := int(^uint(0) >> 1)
			lastBottom := -firstTop
			lastBaseline := -1
			for _, line := range layout.Text {
				bounds, _ := font.BoundString(line.face, line.text)
				top := line.baseline + bounds.Min.Y.Floor()
				bottom := line.baseline + bounds.Max.Y.Ceil()
				if top < firstTop {
					firstTop = top
				}
				if line.baseline > lastBaseline {
					lastBaseline = line.baseline
					lastBottom = bottom
				} else if line.baseline == lastBaseline && bottom > lastBottom {
					lastBottom = bottom
				}
			}
			wantTopGap := metrics.textGap + metrics.framePad
			if got := firstTop - layout.PhotoBounds.Max.Y; got < wantTopGap || got > wantTopGap+1 {
				t.Errorf("photo-to-first-line gap = %d, want %d or %d", got, wantTopGap, wantTopGap+1)
			}
			if got := layout.CanvasBounds.Max.Y - lastBottom; got != metrics.textGap {
				t.Errorf("last-line-to-bottom gap = %d, want %d", got, metrics.textGap)
			}
		})
	}
}

func TestWrapTextRespectsWidth(t *testing.T) {
	face, err := truetype.Parse(gomonobold.TTF)
	if err != nil {
		t.Fatal(err)
	}
	textFace := truetype.NewFace(face, &truetype.Options{Size: 24})
	defer textFace.Close()
	const maxWidth = 110
	lines := wrapText("LongCameraModelName 50mm f/2.8 ISO400", textFace, maxWidth)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %q", lines)
	}
	for _, line := range lines {
		if got := font.MeasureString(textFace, line).Ceil(); got > maxWidth {
			t.Errorf("wrapped line width = %d, exceeds %d: %q", got, maxWidth, line)
		}
	}
}
