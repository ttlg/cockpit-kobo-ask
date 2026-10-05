package main

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
)

var (
	colorBlack     = color.RGBA{0x00, 0x00, 0x00, 0xFF}
	colorWhite     = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	colorGray      = color.RGBA{0x66, 0x66, 0x66, 0xFF}
	colorLightGray = color.RGBA{0xC8, 0xC8, 0xC8, 0xFF}
	colorRule      = color.RGBA{0x99, 0x99, 0x99, 0xFF}
	colorSelected  = color.RGBA{0xB8, 0xD8, 0xFF, 0xFF}
	colorAccent    = color.RGBA{0x2F, 0x6F, 0xB5, 0xFF}
	colorSubmit    = color.RGBA{0x8C, 0xC0, 0xFF, 0xFF}
	colorDanger    = color.RGBA{0xFF, 0xC4, 0xC4, 0xFF}
)

const (
	sizeHeading     = 40
	sizeMeta        = 28
	sizeSummary     = 38
	sizeBody        = 34
	sizeNote        = 28
	sizeButton      = 34
	sizeDescription = 27
	buttonPadding   = 22
	borderWidth     = 3
	minButtonHeight = 104
)

type accent int

const (
	accentNone accent = iota
	accentSelected
	accentSubmit
	accentDanger
)

type iconKind int

const (
	iconNone iconKind = iota
	iconRadio
	iconCheckbox
	iconEdit
)

const (
	iconSize = 40
	iconGap  = 20
)

type hitArea struct {
	rect   image.Rectangle
	action func()
}

type buttonSpec struct {
	icon        iconKind
	iconOn      bool
	label       string
	description string
	accent      accent
	disabled    bool
	plain       bool
	bold        bool
	centered    bool
	weight      int
	action      func()
}

func fillRect(args fillArgs) {
	draw.Draw(args.target, args.rect, image.NewUniform(args.color), image.Point{}, draw.Src)
}

type fillArgs struct {
	target *image.RGBA
	rect   image.Rectangle
	color  color.Color
}

func strokeRect(args strokeArgs) {
	r := args.rect
	fillRect(fillArgs{target: args.target, rect: image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+args.width), color: args.color})
	fillRect(fillArgs{target: args.target, rect: image.Rect(r.Min.X, r.Max.Y-args.width, r.Max.X, r.Max.Y), color: args.color})
	fillRect(fillArgs{target: args.target, rect: image.Rect(r.Min.X, r.Min.Y, r.Min.X+args.width, r.Max.Y), color: args.color})
	fillRect(fillArgs{target: args.target, rect: image.Rect(r.Max.X-args.width, r.Min.Y, r.Max.X, r.Max.Y), color: args.color})
}

type strokeArgs struct {
	target *image.RGBA
	rect   image.Rectangle
	width  int
	color  color.Color
}

func buttonColors(spec buttonSpec) (color.Color, color.Color, color.Color) {
	if spec.disabled {
		return colorWhite, colorLightGray, colorGray
	}
	switch spec.accent {
	case accentSelected:
		return colorSelected, colorAccent, colorBlack
	case accentSubmit:
		return colorSubmit, colorAccent, colorBlack
	case accentDanger:
		return colorDanger, colorBlack, colorBlack
	}
	return colorWhite, colorBlack, colorBlack
}

type layout struct {
	fonts  *fonts
	width  int
	height int
	ops    []func(target *image.RGBA, offsetY int)
	hits   []hitArea
}

func newLayout(args newLayoutArgs) *layout {
	return &layout{fonts: args.fonts, width: args.width}
}

type newLayoutArgs struct {
	fonts *fonts
	width int
}

func (l *layout) gap(height int) {
	l.height += height
}

func (l *layout) text(args layoutTextArgs) {
	face := l.fonts.face(fontKey{bold: args.style.bold, size: args.style.size})
	lines := wrapText(wrapArgs{text: args.text, face: face, width: l.width - args.indent})
	top := l.height
	l.ops = append(l.ops, func(target *image.RGBA, offsetY int) {
		drawLines(drawLinesArgs{target: target, fonts: l.fonts, style: args.style, lines: lines, x: args.indent, y: top + offsetY})
	})
	l.height += len(lines) * lineHeight(args.style.size)
}

type layoutTextArgs struct {
	text   string
	style  textStyle
	indent int
}

func (l *layout) separator() {
	top := l.height
	l.ops = append(l.ops, func(target *image.RGBA, offsetY int) {
		fillRect(fillArgs{target: target, rect: image.Rect(0, top+offsetY, l.width, top+offsetY+2), color: colorRule})
	})
	l.height += 2
}

func measureButton(args measureButtonArgs) (int, []string, []string) {
	labelFace := args.fonts.face(fontKey{bold: args.spec.bold, size: sizeButton})
	descriptionFace := args.fonts.face(fontKey{size: sizeDescription})
	inner := args.width - 2*buttonPadding - iconWidth(args.spec)
	labelLines := wrapText(wrapArgs{text: args.spec.label, face: labelFace, width: inner})
	descriptionLines := []string{}
	if args.spec.description != "" {
		descriptionLines = wrapText(wrapArgs{text: args.spec.description, face: descriptionFace, width: inner})
	}
	height := 2*buttonPadding + len(labelLines)*lineHeight(sizeButton) + len(descriptionLines)*lineHeight(sizeDescription)
	return max(height, args.minHeight), labelLines, descriptionLines
}

type measureButtonArgs struct {
	fonts     *fonts
	spec      buttonSpec
	width     int
	minHeight int
}

func drawButton(args drawButtonArgs) {
	background, border, foreground := buttonColors(args.spec)
	fillRect(fillArgs{target: args.target, rect: args.rect, color: background})
	if !args.spec.plain {
		strokeRect(strokeArgs{target: args.target, rect: args.rect, width: borderWidth, color: border})
	}
	labelFace := args.fonts.face(fontKey{bold: args.spec.bold, size: sizeButton})
	contentHeight := len(args.labelLines)*lineHeight(sizeButton) + len(args.descriptionLines)*lineHeight(sizeDescription)
	y := args.rect.Min.Y + (args.rect.Dy()-contentHeight)/2
	if args.spec.icon != iconNone {
		iconTop := y + (lineHeight(sizeButton)-iconSize)/2
		drawIcon(drawIconArgs{target: args.target, kind: args.spec.icon, on: args.spec.iconOn, rect: image.Rect(args.rect.Min.X+buttonPadding, iconTop, args.rect.Min.X+buttonPadding+iconSize, iconTop+iconSize), color: foreground})
	}
	textLeft := args.rect.Min.X + buttonPadding + iconWidth(args.spec)
	for _, line := range args.labelLines {
		x := textLeft
		if args.spec.centered {
			x = args.rect.Min.X + (args.rect.Dx()-measureWidth(measureWidthArgs{face: labelFace, text: line}))/2
		}
		drawLines(drawLinesArgs{target: args.target, fonts: args.fonts, style: textStyle{size: sizeButton, bold: args.spec.bold, color: foreground}, lines: []string{line}, x: x, y: y})
		y += lineHeight(sizeButton)
	}
	descriptionColor := colorGray
	if args.spec.disabled {
		descriptionColor = colorLightGray
	}
	drawLines(drawLinesArgs{target: args.target, fonts: args.fonts, style: textStyle{size: sizeDescription, color: descriptionColor}, lines: args.descriptionLines, x: textLeft, y: y})
}

type drawButtonArgs struct {
	target           *image.RGBA
	fonts            *fonts
	spec             buttonSpec
	rect             image.Rectangle
	labelLines       []string
	descriptionLines []string
}

func (l *layout) button(spec buttonSpec) {
	height, labelLines, descriptionLines := measureButton(measureButtonArgs{fonts: l.fonts, spec: spec, width: l.width, minHeight: minButtonHeight})
	top := l.height
	l.ops = append(l.ops, func(target *image.RGBA, offsetY int) {
		drawButton(drawButtonArgs{target: target, fonts: l.fonts, spec: spec, rect: image.Rect(0, top+offsetY, l.width, top+offsetY+height), labelLines: labelLines, descriptionLines: descriptionLines})
	})
	if !spec.disabled && spec.action != nil {
		l.hits = append(l.hits, hitArea{rect: image.Rect(0, top, l.width, top+height), action: spec.action})
	}
	l.height += height
}

func (l *layout) render() *image.RGBA {
	target := image.NewRGBA(image.Rect(0, 0, l.width, max(l.height, 1)))
	fillRect(fillArgs{target: target, rect: target.Bounds(), color: colorWhite})
	for _, op := range l.ops {
		op(target, 0)
	}
	return target
}

func measureWidth(args measureWidthArgs) int {
	return font.MeasureString(args.face, args.text).Ceil()
}

type measureWidthArgs struct {
	face font.Face
	text string
}

func iconWidth(spec buttonSpec) int {
	if spec.icon == iconNone {
		return 0
	}
	return iconSize + iconGap
}

func fillCircle(args circleArgs) {
	radiusSquared := args.radius * args.radius
	for y := -args.radius; y <= args.radius; y++ {
		for x := -args.radius; x <= args.radius; x++ {
			if x*x+y*y <= radiusSquared {
				args.target.Set(args.center.X+x, args.center.Y+y, args.color)
			}
		}
	}
}

type circleArgs struct {
	target *image.RGBA
	center image.Point
	radius int
	color  color.Color
}

func drawThickLine(args lineArgs) {
	steps := max(abs(args.to.X-args.from.X), abs(args.to.Y-args.from.Y), 1)
	for step := 0; step <= steps; step++ {
		point := image.Point{args.from.X + (args.to.X-args.from.X)*step/steps, args.from.Y + (args.to.Y-args.from.Y)*step/steps}
		fillCircle(circleArgs{target: args.target, center: point, radius: args.width / 2, color: args.color})
	}
}

type lineArgs struct {
	target *image.RGBA
	from   image.Point
	to     image.Point
	width  int
	color  color.Color
}

func drawIcon(args drawIconArgs) {
	r := args.rect
	center := image.Point{(r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2}
	switch args.kind {
	case iconRadio:
		fillCircle(circleArgs{target: args.target, center: center, radius: iconSize / 2, color: args.color})
		fillCircle(circleArgs{target: args.target, center: center, radius: iconSize/2 - 4, color: colorWhite})
		if args.on {
			fillCircle(circleArgs{target: args.target, center: center, radius: iconSize/2 - 10, color: args.color})
		}
	case iconCheckbox:
		strokeRect(strokeArgs{target: args.target, rect: r, width: 4, color: args.color})
		if args.on {
			fillRect(fillArgs{target: args.target, rect: r, color: args.color})
			drawThickLine(lineArgs{target: args.target, from: image.Point{r.Min.X + 8, center.Y + 1}, to: image.Point{r.Min.X + 16, r.Max.Y - 10}, width: 6, color: colorWhite})
			drawThickLine(lineArgs{target: args.target, from: image.Point{r.Min.X + 16, r.Max.Y - 10}, to: image.Point{r.Max.X - 7, r.Min.Y + 9}, width: 6, color: colorWhite})
		}
	case iconEdit:
		drawThickLine(lineArgs{target: args.target, from: image.Point{r.Min.X + 10, r.Max.Y - 10}, to: image.Point{r.Max.X - 6, r.Min.Y + 6}, width: 8, color: args.color})
		drawThickLine(lineArgs{target: args.target, from: image.Point{r.Min.X + 4, r.Max.Y - 4}, to: image.Point{r.Min.X + 10, r.Max.Y - 10}, width: 4, color: args.color})
		fillRect(fillArgs{target: args.target, rect: image.Rect(r.Min.X+16, r.Max.Y-3, r.Max.X, r.Max.Y), color: args.color})
	}
}

type drawIconArgs struct {
	target *image.RGBA
	kind   iconKind
	on     bool
	rect   image.Rectangle
	color  color.Color
}
