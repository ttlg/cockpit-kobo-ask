package main

import (
	"image"
	"image/color"
	"strings"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const noLineStart = "、。，．,.・：；？！?!）)」』】〉》ー々ぁぃぅぇぉっゃゅょゎァィゥェォッャュョヮ…"

type fontKey struct {
	bold bool
	size int
}

type fonts struct {
	regular *opentype.Font
	bold    *opentype.Font
	faces   map[fontKey]font.Face
}

func loadFonts(args loadFontsArgs) (*fonts, error) {
	regular, err := opentype.Parse(args.regular)
	if err != nil {
		return nil, err
	}
	bold, err := opentype.Parse(args.bold)
	if err != nil {
		return nil, err
	}
	return &fonts{regular: regular, bold: bold, faces: map[fontKey]font.Face{}}, nil
}

type loadFontsArgs struct {
	regular []byte
	bold    []byte
}

func (f *fonts) face(key fontKey) font.Face {
	if face, ok := f.faces[key]; ok {
		return face
	}
	source := f.regular
	if key.bold {
		source = f.bold
	}
	face, err := opentype.NewFace(source, &opentype.FaceOptions{Size: float64(key.size), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	f.faces[key] = face
	return face
}

type textStyle struct {
	size  int
	bold  bool
	color color.Color
}

func lineHeight(size int) int {
	return size * 3 / 2
}

func isBreakableAnywhere(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) || (r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF)
}

func wrapText(args wrapArgs) []string {
	lines := []string{}
	for _, paragraph := range strings.Split(args.text, "\n") {
		lines = append(lines, wrapParagraph(wrapArgs{text: paragraph, face: args.face, width: args.width})...)
	}
	return lines
}

type wrapArgs struct {
	text  string
	face  font.Face
	width int
}

func wrapParagraph(args wrapArgs) []string {
	runes := []rune(args.text)
	if len(runes) == 0 {
		return []string{""}
	}
	lines := []string{}
	start := 0
	lastBreak := -1
	widthLimit := fixed.I(args.width)
	for index := 0; index < len(runes); index++ {
		if index > start && (runes[index-1] == ' ' || isBreakableAnywhere(runes[index-1]) || isBreakableAnywhere(runes[index])) {
			lastBreak = index
		}
		if font.MeasureString(args.face, string(runes[start:index+1])) <= widthLimit || index == start {
			continue
		}
		cut := index
		if lastBreak > start {
			cut = lastBreak
		}
		for cut < len(runes) && strings.ContainsRune(noLineStart, runes[cut]) && cut-start > 1 {
			cut++
		}
		if cut > len(runes) {
			cut = len(runes)
		}
		lines = append(lines, strings.TrimRight(string(runes[start:cut]), " "))
		start = cut
		for start < len(runes) && runes[start] == ' ' {
			start++
		}
		lastBreak = -1
		index = start - 1
	}
	if start < len(runes) {
		lines = append(lines, string(runes[start:]))
	}
	return lines
}

func drawLines(args drawLinesArgs) {
	face := args.fonts.face(fontKey{bold: args.style.bold, size: args.style.size})
	drawer := &font.Drawer{Dst: args.target, Src: image.NewUniform(args.style.color), Face: face}
	ascent := face.Metrics().Ascent.Ceil()
	for index, line := range args.lines {
		drawer.Dot = fixed.P(args.x, args.y+index*lineHeight(args.style.size)+ascent+(lineHeight(args.style.size)-face.Metrics().Height.Ceil())/2)
		drawer.DrawString(line)
	}
}

type drawLinesArgs struct {
	target *image.RGBA
	fonts  *fonts
	style  textStyle
	lines  []string
	x      int
	y      int
}

func ellipsize(args wrapArgs) string {
	if font.MeasureString(args.face, args.text) <= fixed.I(args.width) {
		return args.text
	}
	runes := []rune(args.text)
	for length := len(runes) - 1; length > 0; length-- {
		candidate := string(runes[:length]) + "…"
		if font.MeasureString(args.face, candidate) <= fixed.I(args.width) {
			return candidate
		}
	}
	return "…"
}
