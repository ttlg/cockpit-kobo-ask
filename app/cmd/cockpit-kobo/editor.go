package main

import (
	"image"
	"slices"
	"strings"
)

const (
	keyHeight = 118
	keyGap    = 8
)

type editorTarget struct {
	askID         string
	questionIndex int
}

type editor struct {
	target  editorTarget
	title   string
	hint    string
	text    []rune
	shift   bool
	symbols bool
	hits    []hitArea
}

type keySpec struct {
	label  string
	weight int
	action func(e *editor)
}

func (a *app) openEditor(target editorTarget) {
	index := slices.IndexFunc(a.asks, func(item ask) bool { return item.ID == target.askID })
	if index < 0 {
		return
	}
	item := a.asks[index]
	d := a.draftOf(item)
	e := &editor{target: target, title: a.tr.text("whole_title"), hint: a.tr.text("whole_placeholder"), text: []rune(d.whole)}
	if target.questionIndex >= 0 {
		e.title = item.Questions[target.questionIndex].Title
		if e.title == "" {
			e.title = headingOf(headingArgs{ask: item, tr: a.tr})
		}
		e.hint = a.tr.text("input_placeholder")
		e.text = []rune(d.inputs[target.questionIndex])
	}
	a.editor = e
	a.render(refreshFull)
}

func (a *app) editorTargetOpen() bool {
	return slices.ContainsFunc(a.asks, func(item ask) bool { return item.ID == a.editor.target.askID })
}

func (a *app) finishEditor(save bool) {
	e := a.editor
	a.editor = nil
	index := slices.IndexFunc(a.asks, func(item ask) bool { return item.ID == e.target.askID })
	if save && index >= 0 {
		d := a.draftOf(a.asks[index])
		if e.target.questionIndex >= 0 {
			d.inputs[e.target.questionIndex] = string(e.text)
		} else {
			d.whole = string(e.text)
		}
	}
	a.render(refreshFull)
}

func letterKey(label string) keySpec {
	return keySpec{label: label, weight: 2, action: func(e *editor) {
		character := label
		if e.shift {
			character = strings.ToUpper(label)
			e.shift = false
		}
		e.text = append(e.text, []rune(character)...)
	}}
}

func characterKeys(characters string) []keySpec {
	keys := []keySpec{}
	for _, character := range characters {
		keys = append(keys, letterKey(string(character)))
	}
	return keys
}

func (e *editor) rows(tr translator) [][]keySpec {
	backspace := keySpec{label: "←", weight: 3, action: func(e *editor) {
		if len(e.text) > 0 {
			e.text = e.text[:len(e.text)-1]
		}
	}}
	space := keySpec{label: tr.text("space"), weight: 8, action: func(e *editor) { e.text = append(e.text, ' ') }}
	newline := keySpec{label: "⏎", weight: 3, action: func(e *editor) { e.text = append(e.text, '\n') }}
	if e.symbols {
		return [][]keySpec{
			characterKeys("1234567890"),
			characterKeys("-/:;()&@\"'"),
			append(append([]keySpec{}, characterKeys("#%*+=_?!")...), backspace),
			{{label: "ABC", weight: 3, action: func(e *editor) { e.symbols = false }}, letterKey(","), space, letterKey("."), newline},
		}
	}
	shiftLabel := "⇧"
	if e.shift {
		shiftLabel = "⬆"
	}
	return [][]keySpec{
		characterKeys("qwertyuiop"),
		characterKeys("asdfghjkl"),
		append(append([]keySpec{{label: shiftLabel, weight: 3, action: func(e *editor) { e.shift = !e.shift }}}, characterKeys("zxcvbnm")...), backspace),
		{{label: "123", weight: 3, action: func(e *editor) { e.symbols = true }}, letterKey(","), space, letterKey("."), newline},
	}
}

func (e *editor) render(a *app) *image.RGBA {
	target := image.NewRGBA(image.Rect(0, 0, a.width(), a.height()))
	fillRect(fillArgs{target: target, rect: target.Bounds(), color: colorWhite})
	e.hits = nil
	headingFace := a.fonts.face(fontKey{bold: true, size: sizeHeading})
	drawLines(drawLinesArgs{target: target, fonts: a.fonts, style: textStyle{size: sizeHeading, bold: true, color: colorBlack}, lines: []string{ellipsize(wrapArgs{text: e.title, face: headingFace, width: a.width() - 2*margin})}, x: margin, y: 36})
	fillRect(fillArgs{target: target, rect: image.Rect(0, headerHeight-3, a.width(), headerHeight), color: colorBlack})
	rows := e.rows(a.tr)
	keyboardTop := a.height() - margin - len(rows)*(keyHeight+keyGap)
	actionTop := keyboardTop - rowHeight - 2*footerGap
	box := image.Rect(margin, headerHeight+margin, a.width()-margin, actionTop-footerGap)
	strokeRect(strokeArgs{target: target, rect: box, width: borderWidth, color: colorBlack})
	bodyFace := a.fonts.face(fontKey{size: sizeBody})
	inner := box.Inset(buttonPadding)
	if len(e.text) == 0 {
		drawLines(drawLinesArgs{target: target, fonts: a.fonts, style: textStyle{size: sizeBody, color: colorGray}, lines: []string{e.hint}, x: inner.Min.X, y: inner.Min.Y})
	}
	lines := wrapText(wrapArgs{text: string(e.text) + "▏", face: bodyFace, width: inner.Dx()})
	visible := max(inner.Dy()/lineHeight(sizeBody), 1)
	if len(lines) > visible {
		lines = lines[len(lines)-visible:]
	}
	drawLines(drawLinesArgs{target: target, fonts: a.fonts, style: textStyle{size: sizeBody, color: colorBlack}, lines: lines, x: inner.Min.X, y: inner.Min.Y})
	a.drawRow(drawRowArgs{target: target, row: []buttonSpec{
		{label: a.tr.text("cancel"), centered: true, action: func() { a.finishEditor(false) }},
		{label: a.tr.text("done"), centered: true, bold: true, accent: accentSubmit, action: func() { a.finishEditor(true) }},
	}, top: actionTop})
	e.hits = append(e.hits, a.hits...)
	a.hits = nil
	y := keyboardTop
	for _, row := range rows {
		totalWeight := 0
		for _, key := range row {
			totalWeight += key.weight
		}
		available := a.width() - 2*margin - keyGap*(len(row)-1)
		x := margin
		for keyIndex, key := range row {
			width := available * key.weight / totalWeight
			if keyIndex == len(row)-1 {
				width = a.width() - margin - x
			}
			rect := image.Rect(x, y, x+width, y+keyHeight)
			spec := buttonSpec{label: key.label, centered: true}
			_, labelLines, _ := measureButton(measureButtonArgs{fonts: a.fonts, spec: spec, width: width, minHeight: keyHeight})
			drawButton(drawButtonArgs{target: target, fonts: a.fonts, spec: spec, rect: rect, labelLines: labelLines[:1]})
			action := key.action
			e.hits = append(e.hits, hitArea{rect: rect, action: func() {
				action(e)
				a.render(refreshPartial)
			}})
			x += width + keyGap
		}
		y += keyHeight + keyGap
	}
	return target
}

func (e *editor) onTap(args editorTapArgs) {
	point := image.Point{args.x, args.y}
	for _, hit := range e.hits {
		if point.In(hit.rect.Inset(-keyGap / 2)) {
			hit.action()
			return
		}
	}
}

type editorTapArgs struct {
	app  *app
	x    int
	y    int
}
