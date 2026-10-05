package main

import (
	"errors"
	"image"
	"image/draw"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	margin       = 44
	headerHeight = 150
	footerGap    = 14
	rowHeight    = 108
)

type pollResult struct {
	asks []ask
	err  error
}

type actionResult struct {
	askID   string
	err     error
	failure string
}

type app struct {
	display   *display
	fonts     *fonts
	relay     relayClient
	tr        translator
	interval  time.Duration
	asks      []ask
	currentID string
	drafts    map[string]*draft
	scroll    int
	status    string
	busy      bool
	editor    *editor
	hits      []hitArea
	viewport  image.Rectangle
	content   *image.RGBA
	exit      chan struct{}
	exitOnce  sync.Once
	results   chan any
	pollNow   chan struct{}
	loaded    bool
}

func (a *app) quit() {
	a.exitOnce.Do(func() { close(a.exit) })
}

func (a *app) width() int {
	return a.display.width
}

func (a *app) height() int {
	return a.display.height
}

func (a *app) currentAsk() (ask, int, bool) {
	index := slices.IndexFunc(a.asks, func(item ask) bool { return item.ID == a.currentID })
	if index < 0 && len(a.asks) > 0 {
		index = 0
		a.currentID = a.asks[0].ID
	}
	if index < 0 {
		return ask{}, -1, false
	}
	return a.asks[index], index, true
}

func (a *app) draftOf(item ask) *draft {
	existing, ok := a.drafts[item.ID]
	if ok && len(existing.choices) == len(item.Questions) {
		return existing
	}
	created := newDraft(item)
	a.drafts[item.ID] = created
	return created
}

func (a *app) applyPoll(result pollResult) refreshMode {
	if result.err != nil {
		a.status = a.relayErrorText(result.err)
		return refreshPartial
	}
	a.status = ""
	previousIDs := idsOf(a.asks)
	a.asks = result.asks
	open := map[string]bool{}
	for _, item := range a.asks {
		open[item.ID] = true
	}
	for id := range a.drafts {
		if !open[id] {
			delete(a.drafts, id)
		}
	}
	if !open[a.currentID] {
		a.currentID = ""
		a.scroll = 0
	}
	if !a.loaded || previousIDs != idsOf(a.asks) {
		a.loaded = true
		return refreshFull
	}
	return refreshPartial
}

func idsOf(items []ask) string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return strings.Join(ids, ",")
}

func (a *app) relayErrorText(err error) string {
	statusErr := relayError{}
	if errors.As(err, &statusErr) && statusErr.status == 401 {
		return a.tr.text("wrong_token")
	}
	return a.tr.text("relay_unreachable", "error", err.Error())
}

func (a *app) buildContent(width int) *layout {
	l := newLayout(newLayoutArgs{fonts: a.fonts, width: width})
	item, index, ok := a.currentAsk()
	if !ok {
		l.gap(160)
		l.text(layoutTextArgs{text: a.tr.text("empty_title"), style: textStyle{size: sizeHeading, bold: true, color: colorBlack}})
		l.gap(20)
		l.text(layoutTextArgs{text: a.tr.text("empty_description"), style: textStyle{size: sizeBody, color: colorGray}})
		return l
	}
	d := a.draftOf(item)
	l.gap(24)
	l.text(layoutTextArgs{text: strconv.Itoa(index+1) + "/" + strconv.Itoa(len(a.asks)) + " · " + item.Time, style: textStyle{size: sizeMeta, color: colorGray}})
	l.gap(12)
	l.text(layoutTextArgs{text: item.Summary, style: textStyle{size: sizeSummary, color: colorBlack}})
	if item.MediaCount > 0 {
		l.gap(16)
		l.text(layoutTextArgs{text: a.tr.text("media_note", "count", strconv.Itoa(item.MediaCount)), style: textStyle{size: sizeNote, color: colorGray}})
	}
	for questionIndex, question := range item.Questions {
		a.questionBlock(questionBlockArgs{layout: l, ask: item, draft: d, questionIndex: questionIndex, question: question})
	}
	if len(item.Questions) > 1 {
		l.gap(28)
		l.separator()
		l.gap(20)
		l.text(layoutTextArgs{text: a.tr.text("whole_title"), style: textStyle{size: sizeBody, bold: true, color: colorBlack}})
		l.gap(8)
		l.text(layoutTextArgs{text: a.tr.text("whole_description"), style: textStyle{size: sizeNote, color: colorGray}})
		l.gap(14)
		l.button(buttonSpec{icon: iconEdit, label: inputLabel(inputLabelArgs{text: d.whole, placeholder: a.tr.text("whole_placeholder")}), action: func() { a.openEditor(editorTarget{askID: item.ID, questionIndex: -1}) }})
	}
	if requiresSubmit(item) {
		summary := describeSubmission(submissionArgs{ask: item, draft: d})
		switch {
		case summary.ready:
			chips := submissionChips(chipArgs{summary: summary, draft: d, tr: a.tr})
			l.gap(24)
			l.text(layoutTextArgs{text: a.tr.text("send_summary", "chips", strings.Join(chips, a.tr.text("chip_separator"))), style: textStyle{size: sizeNote, color: colorGray}})
		case len(item.Questions) > 1:
			l.gap(24)
			l.text(layoutTextArgs{text: a.tr.text("incomplete"), style: textStyle{size: sizeNote, color: colorGray}})
		}
	}
	l.gap(40)
	return l
}

type questionBlockArgs struct {
	layout        *layout
	ask           ask
	draft         *draft
	questionIndex int
	question      askQuestion
}

func (a *app) questionBlock(args questionBlockArgs) {
	l := args.layout
	l.gap(28)
	if len(args.ask.Questions) > 1 {
		l.separator()
		l.gap(20)
	}
	header := args.question.Title
	if args.question.Multiple && len(args.question.Choices) > 0 {
		badge := a.tr.text("badge_multiple")
		if count := len(args.draft.choices[args.questionIndex]); count > 0 {
			badge = a.tr.text("badge_selected", "count", strconv.Itoa(count))
		}
		header = strings.TrimSpace(header + "  [" + badge + "]")
	}
	if header != "" {
		l.text(layoutTextArgs{text: header, style: textStyle{size: sizeBody, bold: true, color: colorBlack}})
		l.gap(14)
	}
	tapToSend := isTapToSend(args.ask)
	for choiceIndex, choice := range args.question.Choices {
		number := choiceIndex + 1
		selected := slices.Contains(args.draft.choices[args.questionIndex], number)
		description := ""
		if choiceIndex < len(args.question.ChoiceDescriptions) {
			description = args.question.ChoiceDescriptions[choiceIndex]
		}
		spec := buttonSpec{label: choice, description: description, iconOn: selected, icon: choiceIcon(choiceIconArgs{tapToSend: tapToSend, multiple: args.question.Multiple}), action: func() {
			a.onChoice(onChoiceArgs{ask: args.ask, questionIndex: args.questionIndex, choiceIndex: number})
		}}
		if selected {
			spec.accent = accentSelected
		}
		l.button(spec)
		l.gap(12)
	}
	if args.question.AllowInput {
		l.button(buttonSpec{icon: iconEdit, label: inputLabel(inputLabelArgs{text: args.draft.inputs[args.questionIndex], placeholder: a.tr.text("input_placeholder")}), action: func() {
			a.openEditor(editorTarget{askID: args.ask.ID, questionIndex: args.questionIndex})
		}})
	}
}

func choiceIcon(args choiceIconArgs) iconKind {
	switch {
	case args.tapToSend:
		return iconNone
	case args.multiple:
		return iconCheckbox
	}
	return iconRadio
}

type choiceIconArgs struct {
	tapToSend bool
	multiple  bool
}

func inputLabel(args inputLabelArgs) string {
	text := strings.Join(strings.Fields(args.text), " ")
	if text == "" {
		return args.placeholder
	}
	runes := []rune(text)
	if len(runes) > 60 {
		text = string(runes[:60]) + "…"
	}
	return text
}

type inputLabelArgs struct {
	text        string
	placeholder string
}

func (a *app) footerRows() [][]buttonSpec {
	rows := [][]buttonSpec{}
	item, index, ok := a.currentAsk()
	if ok {
		if hasSubmitButton(item) {
			label := a.tr.text("send")
			if a.busy {
				label = a.tr.text("sending")
			}
			ready := isSubmitReady(submissionArgs{ask: item, draft: a.draftOf(item)}) && !a.busy
			rows = append(rows, []buttonSpec{{label: label, bold: true, centered: true, accent: accentSubmit, disabled: !ready, action: func() { a.submit(item) }}})
		}
		count := a.tr.text("single")
		if len(a.asks) > 1 {
			count = a.tr.text("queue", "count", strconv.Itoa(len(a.asks)))
		}
		rows = append(rows, []buttonSpec{
			{label: a.tr.text("close"), weight: 2, centered: true, accent: accentDanger, disabled: a.busy, action: func() { a.closeAsk(item) }},
			{label: a.tr.text("previous"), weight: 2, centered: true, disabled: index == 0, action: func() { a.move(-1) }},
			{label: count, weight: 3, centered: true, plain: true, disabled: true},
			{label: a.tr.text("next"), weight: 2, centered: true, disabled: index >= len(a.asks)-1, action: func() { a.move(1) }},
		})
	}
	rows = append(rows, []buttonSpec{{label: a.tr.text("kobo_home"), centered: true, action: a.quit}})
	return rows
}

func (a *app) renderMain() *image.RGBA {
	target := image.NewRGBA(image.Rect(0, 0, a.width(), a.height()))
	fillRect(fillArgs{target: target, rect: target.Bounds(), color: colorWhite})
	a.hits = nil
	heading := "AGI Cockpit"
	if item, _, ok := a.currentAsk(); ok {
		heading = headingOf(headingArgs{ask: item, tr: a.tr})
	}
	headingFace := a.fonts.face(fontKey{bold: true, size: sizeHeading})
	drawLines(drawLinesArgs{target: target, fonts: a.fonts, style: textStyle{size: sizeHeading, bold: true, color: colorBlack}, lines: []string{ellipsize(wrapArgs{text: heading, face: headingFace, width: a.width() - 2*margin})}, x: margin, y: 36})
	if a.status != "" {
		statusFace := a.fonts.face(fontKey{size: sizeMeta})
		drawLines(drawLinesArgs{target: target, fonts: a.fonts, style: textStyle{size: sizeMeta, color: colorAccent}, lines: []string{ellipsize(wrapArgs{text: a.status, face: statusFace, width: a.width() - 2*margin})}, x: margin, y: 36 + lineHeight(sizeHeading)})
	}
	fillRect(fillArgs{target: target, rect: image.Rect(0, headerHeight-3, a.width(), headerHeight), color: colorBlack})
	rows := a.footerRows()
	footerTop := a.height() - footerGap - len(rows)*(rowHeight+footerGap)
	fillRect(fillArgs{target: target, rect: image.Rect(0, footerTop-3, a.width(), footerTop), color: colorBlack})
	y := footerTop + footerGap
	for _, row := range rows {
		a.drawRow(drawRowArgs{target: target, row: row, top: y})
		y += rowHeight + footerGap
	}
	a.viewport = image.Rect(margin, headerHeight, a.width()-margin, footerTop-3)
	l := a.buildContent(a.viewport.Dx())
	a.content = l.render()
	maxScroll := max(a.content.Bounds().Dy()-a.viewport.Dy(), 0)
	a.scroll = min(max(a.scroll, 0), maxScroll)
	draw.Draw(target, a.viewport, a.content, image.Point{0, a.scroll}, draw.Src)
	for _, hit := range l.hits {
		rect := hit.rect.Add(image.Point{a.viewport.Min.X, a.viewport.Min.Y - a.scroll}).Intersect(a.viewport)
		if !rect.Empty() {
			a.hits = append(a.hits, hitArea{rect: rect, action: hit.action})
		}
	}
	if maxScroll > 0 {
		trackHeight := a.viewport.Dy()
		thumbHeight := max(trackHeight*a.viewport.Dy()/a.content.Bounds().Dy(), 60)
		thumbTop := a.viewport.Min.Y + (trackHeight-thumbHeight)*a.scroll/maxScroll
		fillRect(fillArgs{target: target, rect: image.Rect(a.width()-22, a.viewport.Min.Y, a.width()-18, a.viewport.Max.Y), color: colorLightGray})
		fillRect(fillArgs{target: target, rect: image.Rect(a.width()-26, thumbTop, a.width()-14, thumbTop+thumbHeight), color: colorBlack})
	}
	return target
}

func (a *app) drawRow(args drawRowArgs) {
	gap := 12
	totalWeight := 0
	for _, spec := range args.row {
		totalWeight += max(spec.weight, 1)
	}
	available := a.width() - 2*margin - gap*(len(args.row)-1)
	left := margin
	for index, spec := range args.row {
		width := available * max(spec.weight, 1) / totalWeight
		if index == len(args.row)-1 {
			width = a.width() - margin - left
		}
		rect := image.Rect(left, args.top, left+width, args.top+rowHeight)
		_, labelLines, _ := measureButton(measureButtonArgs{fonts: a.fonts, spec: spec, width: width, minHeight: rowHeight})
		drawButton(drawButtonArgs{target: args.target, fonts: a.fonts, spec: spec, rect: rect, labelLines: labelLines[:min(len(labelLines), 2)]})
		if !spec.disabled && spec.action != nil {
			a.hits = append(a.hits, hitArea{rect: rect, action: spec.action})
		}
		left += width + gap
	}
}

type drawRowArgs struct {
	target *image.RGBA
	row    []buttonSpec
	top    int
}

func (a *app) render(mode refreshMode) {
	if a.editor != nil && !a.editorTargetOpen() {
		a.editor = nil
	}
	var frame *image.RGBA
	if a.editor != nil {
		frame = a.editor.render(a)
	} else {
		frame = a.renderMain()
	}
	if err := a.display.present(presentArgs{image: frame, mode: mode}); err != nil {
		log.Println("present:", err)
	}
}

func (a *app) move(step int) {
	_, index, ok := a.currentAsk()
	if !ok || index+step < 0 || index+step >= len(a.asks) {
		return
	}
	a.currentID = a.asks[index+step].ID
	a.scroll = 0
	a.render(refreshFull)
}

func (a *app) onChoice(args onChoiceArgs) {
	d := a.draftOf(args.ask)
	if isTapToSend(args.ask) {
		d.choices[0] = []int{args.choiceIndex}
		a.submit(args.ask)
		return
	}
	toggleChoice(toggleArgs{ask: args.ask, draft: d, questionIndex: args.questionIndex, choiceIndex: args.choiceIndex})
	a.render(refreshPartial)
}

type onChoiceArgs struct {
	ask           ask
	questionIndex int
	choiceIndex   int
}

func (a *app) submit(item ask) {
	if a.busy {
		return
	}
	request := buildAnswerRequest(submissionArgs{ask: item, draft: a.draftOf(item)})
	a.busy = true
	a.render(refreshPartial)
	go func() {
		err := a.relay.answer(answerArgs{askID: item.ID, request: request})
		a.results <- actionResult{askID: item.ID, err: err, failure: "send_failed"}
	}()
}

func (a *app) closeAsk(item ask) {
	if a.busy {
		return
	}
	a.busy = true
	a.render(refreshPartial)
	go func() {
		err := a.relay.closeAsk(item.ID)
		a.results <- actionResult{askID: item.ID, err: err, failure: "close_failed"}
	}()
}

func (a *app) applyAction(result actionResult) {
	a.busy = false
	if result.err != nil {
		a.status = a.tr.text(result.failure, "error", result.err.Error())
		a.render(refreshPartial)
		return
	}
	a.status = ""
	a.asks = slices.DeleteFunc(a.asks, func(item ask) bool { return item.ID == result.askID })
	delete(a.drafts, result.askID)
	a.currentID = ""
	a.scroll = 0
	a.render(refreshFull)
	select {
	case a.pollNow <- struct{}{}:
	default:
	}
}

func (a *app) onGesture(g gesture) {
	if a.busy {
		return
	}
	if a.editor != nil {
		a.editor.onTap(editorTapArgs{app: a, x: g.x, y: g.y})
		return
	}
	point := image.Point{g.x, g.y}
	if g.kind == gestureSwipe && point.In(a.viewport) {
		a.scrollBy(-g.dy)
		return
	}
	for _, hit := range a.hits {
		if point.In(hit.rect) {
			hit.action()
			return
		}
	}
}

func (a *app) scrollBy(delta int) {
	previous := a.scroll
	a.scroll += delta
	maxScroll := 0
	if a.content != nil {
		maxScroll = max(a.content.Bounds().Dy()-a.viewport.Dy(), 0)
	}
	a.scroll = min(max(a.scroll, 0), maxScroll)
	if a.scroll != previous {
		a.render(refreshPartial)
	}
}

func (a *app) onKey(key keyPress) {
	switch key.code {
	case keyPower:
		a.quit()
	case keyPageDown, 194:
		if a.editor == nil {
			a.scrollBy(a.viewport.Dy() * 4 / 5)
		}
	case keyPageUp, 193:
		if a.editor == nil {
			a.scrollBy(-a.viewport.Dy() * 4 / 5)
		}
	}
	log.Println("key:", key.code)
}

func (a *app) poller() {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		asks, err := a.relay.listAsks()
		a.results <- pollResult{asks: asks, err: err}
		select {
		case <-ticker.C:
		case <-a.pollNow:
		case <-a.exit:
			return
		}
	}
}

func (a *app) run(args runArgs) {
	a.render(refreshFull)
	go a.poller()
	for {
		select {
		case <-a.exit:
			return
		case g := <-args.gestures:
			a.onGesture(g)
		case key := <-args.keys:
			a.onKey(key)
		case err := <-args.inputErrors:
			log.Println("input:", err)
			return
		case result := <-a.results:
			switch value := result.(type) {
			case pollResult:
				if a.busy {
					continue
				}
				mode := a.applyPoll(value)
				if a.editor == nil || mode == refreshFull {
					a.render(mode)
				}
			case actionResult:
				a.applyAction(value)
			}
		}
	}
}

type runArgs struct {
	gestures    <-chan gesture
	keys        <-chan keyPress
	inputErrors <-chan error
}
