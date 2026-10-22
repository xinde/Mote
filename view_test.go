package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestViewRendersEditorAndPreview(t *testing.T) {
	draft := "# 周五出发\n\n- [x] 订车票\n- [ ] 收拾充电器\n\n记得带上 **护照**。"
	updated := time.Date(2026, 10, 16, 10, 30, 0, 0, time.Local)
	s := newApp(&store{path: "/Users/test/Library/Application Support/Mote/notes.db"},
		note{ID: 4, Markdown: draft, UpdatedAt: updated},
		[]noteSummary{{ID: 4, Title: "周五出发", UpdatedAt: updated}},
		defaultTimerState(),
		0,
	)

	tester := ui.NewTester(s.view, 460, 540)
	if tester.HasText("关闭 Mote") || tester.HasText("×") {
		t.Fatalf("header still shows a close control: %q", tester.Texts())
	}
	for _, text := range []string{"Mote", "记录", "专注", "编辑", "预览", "已保存 · 10:30", "立即保存"} {
		if !tester.HasText(text) {
			t.Errorf("editor view missing %q; texts: %q", text, tester.Texts())
		}
	}
	if tester.HasText("notes.db") {
		t.Errorf("footer still shows a separate database label: %q", tester.Texts())
	}
	if tester.HasText("⌘S 保存  ·  ⎋ 收起  ·  ⇧⌘M 呼出") {
		t.Errorf("footer still shows the shortcut line: %q", tester.Texts())
	}
	if err := tester.Click("已保存 · 10:30"); err != nil {
		t.Fatal(err)
	}
	if want := "/Users/test/Library/Application Support/Mote/notes.db"; tester.Clipboard() != want {
		t.Errorf("clipboard %q, want %q", tester.Clipboard(), want)
	}
	if err := tester.Click("预览"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"周五出发", "订车票", "护照"} {
		if !tester.HasText(text) {
			t.Errorf("preview missing %q; texts: %q", text, tester.Texts())
		}
	}
	if err := tester.Click("专注"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"25:00", "今天完成 0 个番茄", "开始专注", "休息", "分钟"} {
		if !tester.HasText(text) {
			t.Errorf("timer view missing %q; texts: %q", text, tester.Texts())
		}
	}
	tester.SetSize(400, 440)
	for _, text := range []string{"25:00", "开始专注", "专注", "休息"} {
		if !tester.HasText(text) {
			t.Errorf("compact timer view missing %q; texts: %q", text, tester.Texts())
		}
	}

	if output := os.Getenv("MOTE_SCREENSHOT"); output != "" {
		file, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, tester.Image()); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
