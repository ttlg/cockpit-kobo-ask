package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderPreview(t *testing.T) {
	output := os.Getenv("PREVIEW_DIR")
	if output == "" {
		t.Skip("set PREVIEW_DIR to write preview images")
	}
	regular, err := os.ReadFile("../../fonts/NotoSansJP-Regular.otf")
	if err != nil {
		t.Fatal(err)
	}
	bold, err := os.ReadFile("../../fonts/NotoSansJP-Bold.otf")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadFonts(loadFontsArgs{regular: regular, bold: bold})
	if err != nil {
		t.Fatal(err)
	}
	title := "Preview"
	directory := "agi-tools"
	sample := ask{ID: "a1", Title: &title, DirectoryLabel: &directory, Time: "10/5 17:00", Summary: "複数の質問がある Ask のテストです。チェックボックスとラジオボタンの描画を確認します。", Questions: []askQuestion{
		{ID: "q1", Title: "単一選択", Choices: []string{"白黒", "カラー"}, ChoiceDescriptions: []string{"くっきり見える", "色が分かる"}, AllowInput: true},
		{ID: "q2", Title: "複数選択", Choices: []string{"通知音", "物理ボタン", "スリープ復帰"}, ChoiceDescriptions: []string{"", "", ""}, Multiple: true},
	}}
	instance := &app{display: &display{width: 1264, height: 1680}, fonts: loaded, tr: translator{language: "ja"}, asks: []ask{sample}, drafts: map[string]*draft{}}
	d := instance.draftOf(sample)
	d.choices[0] = []int{2}
	d.choices[1] = []int{1, 3}
	d.inputs[0] = "hello"
	instance.editor = &editor{target: editorTarget{askID: "a1", questionIndex: 0}, title: "単一選択", hint: instance.tr.text("input_placeholder"), text: []rune("hello")}
	editorFrame := instance.editor.render(instance)
	instance.editor = nil
	frames := map[string]*image.RGBA{"main.png": instance.renderMain(), "editor.png": editorFrame}
	for name, frame := range frames {
		file, err := os.Create(filepath.Join(output, name))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(file, frame)
		file.Close()
	}
}
