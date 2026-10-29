package main

import (
	"image"
	"strings"

	"golang.org/x/image/font"
)

const (
	defaultFrameRatio       = 0.03
	maxFrameRatio           = 0.25
	defaultJPEGQuality      = 100
	minimumTextGap          = 50
	textGapShortSideRatio   = 0.02
	minimumSideInset        = 12
	sideInsetShortSideRatio = 0.01
	minimumFontSize         = 12.0
	largeFontWidthRatio     = 0.035
	largeFontHeightRatio    = 0.09
	smallFontRatio          = 0.58
)

func compact(parts ...string) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return strings.Join(values, " ")
}

type labelText struct {
	text string
	face font.Face
}

type positionedText struct {
	text        string
	face        font.Face
	x, baseline int
}

type LabelLayout struct {
	CanvasBounds image.Rectangle
	PhotoBounds  image.Rectangle
	Text         []positionedText
}

type layoutMetrics struct {
	framePad, textGap, sideInset int
	largeFontSize, smallFontSize float64
}

func calculateLayoutMetrics(width, height int, cfg Config) layoutMetrics {
	shortSide := min(width, height)
	largeFontSize := max(minimumFontSize, min(float64(width)*largeFontWidthRatio, float64(height)*largeFontHeightRatio))
	framePad := int(float64(shortSide) * cfg.frameRatio)
	if cfg.noFrame {
		framePad = 0
	}
	return layoutMetrics{
		framePad:      framePad,
		textGap:       max(minimumTextGap, int(float64(shortSide)*textGapShortSideRatio)),
		sideInset:     min(max(minimumSideInset, int(float64(shortSide)*sideInsetShortSideRatio)), max(1, width/8)),
		largeFontSize: largeFontSize,
		smallFontSize: largeFontSize * smallFontRatio,
	}
}

func buildLabelText(meta *ExifData, cfg Config, bold, regular font.Face) ([]labelText, []labelText) {
	left, right := []labelText{}, []labelText{}
	if !cfg.noModel {
		if v := compact(meta.Make, meta.Model); v != "" {
			left = append(left, labelText{v, bold})
		}
		if v := compact(meta.LensMake, meta.LensModel); v != "" {
			left = append(left, labelText{v, regular})
		}
	}
	shooting := []string{}
	if meta.FocalLength35mm != "" {
		shooting = append(shooting, meta.FocalLength35mm+"mm")
	} else if meta.FocalLength != "" {
		shooting = append(shooting, meta.FocalLength+"mm")
	}
	if meta.FNumber != "" {
		shooting = append(shooting, "f/"+meta.FNumber)
	}
	if meta.ExposureTime != "" {
		shooting = append(shooting, meta.ExposureTime+"s")
	}
	if meta.ISO != "" {
		shooting = append(shooting, "ISO"+meta.ISO)
	}
	if v := strings.Join(shooting, "  "); v != "" {
		right = append(right, labelText{v, bold})
	}
	if meta.DateTimeOriginal != "" {
		right = append(right, labelText{meta.DateTimeOriginal, regular})
	}
	return left, right
}

func wrapText(text string, face font.Face, maxWidth int) []string {
	words := strings.Fields(text)
	if len(words) == 0 || maxWidth < 1 {
		return nil
	}
	lines := []string{}
	line := ""
	for _, word := range words {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if font.MeasureString(face, candidate).Ceil() <= maxWidth {
			line = candidate
			continue
		}
		if line != "" {
			lines = append(lines, line)
			line = ""
		}
		for _, r := range word {
			candidate = line + string(r)
			if line != "" && font.MeasureString(face, candidate).Ceil() > maxWidth {
				lines = append(lines, line)
				line = string(r)
			} else {
				line = candidate
			}
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func layoutLabel(width, height int, metrics layoutMetrics, left, right []labelText) LabelLayout {
	contentWidth := max(1, width-2*metrics.sideInset)
	columnGap := metrics.sideInset
	columnWidth := max(1, (contentWidth-columnGap)/2)
	leftLines := wrapColumn(left, columnWidth)
	rightLines := wrapColumn(right, columnWidth)
	rows := max(len(leftLines), len(rightLines))
	rowAscents, rowDescents := make([]int, rows), make([]int, rows)
	for i := 0; i < rows; i++ {
		for _, column := range [][]labelText{leftLines, rightLines} {
			if i >= len(column) {
				continue
			}
			bounds, _ := font.BoundString(column[i].face, column[i].text)
			rowAscents[i] = max(rowAscents[i], (-bounds.Min.Y).Ceil())
			rowDescents[i] = max(rowDescents[i], bounds.Max.Y.Ceil())
		}
	}
	framePad, gap := metrics.framePad, metrics.textGap
	labelHeight := 0
	if rows > 0 {
		// The first-line gap includes the lower photo frame; the bottom gap is
		// only the label spacing, as configured independently.
		labelHeight = gap + framePad + gap
		for i := range rowAscents {
			labelHeight += rowAscents[i] + rowDescents[i]
		}
		if rows > 1 {
			labelHeight += (rows - 1) * gap
		}
	}
	canvasWidth := width + 2*framePad
	canvasHeight := height + framePad + labelHeight
	result := LabelLayout{
		CanvasBounds: image.Rect(0, 0, canvasWidth, canvasHeight),
		PhotoBounds:  image.Rect(framePad, framePad, framePad+width, framePad+height),
		Text:         make([]positionedText, 0, len(leftLines)+len(rightLines)),
	}
	y := height + framePad + gap + framePad
	for row := 0; row < rows; row++ {
		baseline := y + rowAscents[row]
		if row < len(leftLines) {
			result.Text = append(result.Text, positionedText{leftLines[row].text, leftLines[row].face, framePad + metrics.sideInset, baseline})
		}
		if row < len(rightLines) {
			line := rightLines[row]
			x := framePad + width - metrics.sideInset - font.MeasureString(line.face, line.text).Ceil()
			result.Text = append(result.Text, positionedText{line.text, line.face, x, baseline})
		}
		y += rowAscents[row] + rowDescents[row] + gap
	}
	return result
}

func wrapColumn(items []labelText, width int) []labelText {
	var lines []labelText
	for _, item := range items {
		for _, text := range wrapText(item.text, item.face, width) {
			lines = append(lines, labelText{text, item.face})
		}
	}
	return lines
}
