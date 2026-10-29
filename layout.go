package main

import (
	"fmt"
	"image"
	"strings"

	"golang.org/x/image/font"
)

const (
	overlayNone              = "none"
	overlayFullWidth         = "full-width"
	overlayTopLeft           = "top-left"
	overlayTopRight          = "top-right"
	overlayBottomLeft        = "bottom-left"
	overlayBottomRight       = "bottom-right"
	defaultFrameRatio        = 0.03
	maxFrameRatio            = 0.25
	defaultJPEGQuality       = 100
	minimumTextGap           = 50
	textGapShortSideRatio    = 0.02
	minimumSideInset         = 12
	sideInsetShortSideRatio  = 0.01
	minimumFontSize          = 12.0
	largeFontWidthRatio      = 0.035
	largeFontHeightRatio     = 0.09
	smallFontRatio           = 0.58
	overlayMinimumFontSize   = 7.0
	overlayFontWidthRatio    = 0.025
	overlayFontHeightRatio   = 0.05
	overlayGapShortSideDiv   = 30
	cornerFontShortSideRatio = 0.022
	cornerSmallFontRatio     = 0.70
	cornerSpacingFontRatio   = 0.35
	cornerInsetFontRatio     = 0.50
	cornerMaxWidthRatio      = 0.55
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
	CanvasBounds  image.Rectangle
	PhotoBounds   image.Rectangle
	OverlayBounds image.Rectangle
	Text          []positionedText
}

type layoutMetrics struct {
	framePad, textGap, sideInset int
	largeFontSize, smallFontSize float64
	overlayPosition              string
}

func calculateLayoutMetrics(width, height int, cfg Config) layoutMetrics {
	position := cfg.overlayPosition
	if position == "" {
		position = overlayNone
	}
	shortSide := min(width, height)
	largeFontSize := max(minimumFontSize, min(float64(width)*largeFontWidthRatio, float64(height)*largeFontHeightRatio))
	framePad := int(float64(shortSide) * cfg.frameRatio)
	textGap := max(minimumTextGap, int(float64(shortSide)*textGapShortSideRatio))
	sideInset := min(max(minimumSideInset, int(float64(shortSide)*sideInsetShortSideRatio)), max(1, width/8))
	smallFontSize := largeFontSize * smallFontRatio
	overlayMode := position != overlayNone
	cornerOverlay := overlayMode && position != overlayFullWidth
	if cfg.noFrame || overlayMode {
		framePad = 0
	}
	if position == overlayFullWidth {
		largeFontSize = max(overlayMinimumFontSize, min(float64(width)*overlayFontWidthRatio, float64(height)*overlayFontHeightRatio))
		smallFontSize = largeFontSize * smallFontRatio
		textGap = max(2, min(textGap, shortSide/overlayGapShortSideDiv))
	}
	if cornerOverlay {
		largeFontSize = max(minimumFontSize, float64(shortSide)*cornerFontShortSideRatio)
		smallFontSize = largeFontSize * cornerSmallFontRatio
		textGap = max(4, int(largeFontSize*cornerSpacingFontRatio))
		sideInset = min(max(6, int(largeFontSize*cornerInsetFontRatio)), max(1, width/8))
	}
	return layoutMetrics{
		framePad:        framePad,
		textGap:         textGap,
		sideInset:       sideInset,
		largeFontSize:   largeFontSize,
		smallFontSize:   smallFontSize,
		overlayPosition: position,
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

func layoutLabel(width, height int, metrics layoutMetrics, left, right []labelText) (LabelLayout, error) {
	if metrics.overlayPosition != overlayNone && metrics.overlayPosition != overlayFullWidth {
		return layoutCornerOverlay(width, height, metrics, left, right, metrics.overlayPosition)
	}
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
		if metrics.framePad > 0 {
			labelHeight = gap + framePad + gap
		} else {
			labelHeight = 2 * gap
		}
		for i := range rowAscents {
			labelHeight += rowAscents[i] + rowDescents[i]
		}
		if rows > 1 {
			labelHeight += (rows - 1) * gap
		}
	}
	if metrics.overlayPosition == overlayFullWidth && labelHeight > height {
		return LabelLayout{}, fmt.Errorf("EXIF overlay needs %d vertical pixels but the image is only %d pixels high", labelHeight, height)
	}
	canvasWidth := width + 2*framePad
	canvasHeight := height + framePad + labelHeight
	photoBounds := image.Rect(framePad, framePad, framePad+width, framePad+height)
	var overlayBounds image.Rectangle
	if metrics.overlayPosition != overlayNone {
		canvasWidth, canvasHeight = width, height
		photoBounds = image.Rect(0, 0, width, height)
		if rows > 0 {
			overlayTop := max(0, height-labelHeight)
			overlayBounds = image.Rect(0, overlayTop, width, height)
		}
	}
	result := LabelLayout{
		CanvasBounds:  image.Rect(0, 0, canvasWidth, canvasHeight),
		PhotoBounds:   photoBounds,
		OverlayBounds: overlayBounds,
		Text:          make([]positionedText, 0, len(leftLines)+len(rightLines)),
	}
	y := height + framePad + gap + framePad
	if metrics.overlayPosition == overlayFullWidth {
		y = max(0, height-labelHeight) + gap
	}
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
	return result, nil
}

func layoutCornerOverlay(width, height int, metrics layoutMetrics, left, right []labelText, position string) (LabelLayout, error) {
	items := make([]labelText, 0, len(left)+len(right))
	items = append(items, left...)
	items = append(items, right...)
	maxPanelWidth := max(1, int(float64(width)*cornerMaxWidthRatio)-2*metrics.sideInset)
	lines := wrapColumn(items, maxPanelWidth)
	rows := len(lines)
	ascents, descents := make([]int, rows), make([]int, rows)
	maxTextWidth := 0
	for i, line := range lines {
		bounds, _ := font.BoundString(line.face, line.text)
		ascents[i] = (-bounds.Min.Y).Ceil()
		descents[i] = bounds.Max.Y.Ceil()
		maxTextWidth = max(maxTextWidth, font.MeasureString(line.face, line.text).Ceil())
	}
	if rows == 0 {
		return LabelLayout{CanvasBounds: image.Rect(0, 0, width, height), PhotoBounds: image.Rect(0, 0, width, height)}, nil
	}
	panelPadding := max(4, int(metrics.largeFontSize*cornerInsetFontRatio))
	panelWidth := maxTextWidth + 2*panelPadding
	panelHeight := 2 * panelPadding
	for i := range ascents {
		panelHeight += ascents[i] + descents[i]
	}
	if rows > 1 {
		panelHeight += (rows - 1) * metrics.textGap
	}
	if panelHeight > height || panelWidth+2*metrics.sideInset > width {
		return LabelLayout{}, fmt.Errorf("compact EXIF overlay does not fit in %dx%d image", width, height)
	}
	leftSide := strings.HasSuffix(position, "left")
	topSide := strings.HasPrefix(position, "top")
	panelLeft, panelTop := metrics.sideInset, height-metrics.sideInset-panelHeight
	if !leftSide {
		panelLeft = width - metrics.sideInset - panelWidth
	}
	if topSide {
		panelTop = metrics.sideInset
	}
	panel := image.Rect(panelLeft, panelTop, panelLeft+panelWidth, panelTop+panelHeight)
	result := LabelLayout{
		CanvasBounds:  image.Rect(0, 0, width, height),
		PhotoBounds:   image.Rect(0, 0, width, height),
		OverlayBounds: panel,
		Text:          make([]positionedText, 0, rows),
	}
	y := panel.Min.Y + panelPadding
	for i, line := range lines {
		baseline := y + ascents[i]
		x := panel.Min.X + panelPadding
		if !leftSide {
			x = panel.Max.X - panelPadding - font.MeasureString(line.face, line.text).Ceil()
		}
		result.Text = append(result.Text, positionedText{line.text, line.face, x, baseline})
		y += ascents[i] + descents[i] + metrics.textGap
	}
	return result, nil
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
