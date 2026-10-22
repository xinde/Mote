package main

import (
	"context"
	"image"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// renderedInkRows returns the first and last row that draws ink inside r.
func renderedInkRows(img image.Image, r ui.Rect) ([2]int, bool) {
	top, bottom := -1, -1
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				if top < 0 {
					top = y
				}
				bottom = y
				break
			}
		}
	}
	return [2]int{top, bottom}, top >= 0
}

func TestNotesFooterLayout(t *testing.T) {
	updated := time.Date(2026, 10, 16, 14, 27, 0, 0, time.Local)
	s := newApp(&store{path: "/Users/test/Library/Application Support/Mote/notes.db"},
		note{ID: 4, Markdown: "# 周五出发\n\n记得带上 **护照**。", UpdatedAt: updated},
		[]noteSummary{{ID: 4, Title: "周五出发", UpdatedAt: updated}},
		defaultTimerState(),
		0,
	)
	tester := ui.NewTester(s.view, 460, 540)

	status, ok := tester.Find("已保存 · 14:27")
	if !ok {
		t.Fatalf("status not found; texts: %q", tester.Texts())
	}
	newButton, ok := tester.Find("新建")
	if !ok {
		t.Fatalf("new button not found; texts: %q", tester.Texts())
	}
	pinButton, ok := tester.Find("钉住")
	if !ok {
		t.Fatalf("pin button not found; texts: %q", tester.Texts())
	}
	deleteButton, ok := tester.Find("删除")
	if !ok {
		t.Fatalf("delete button not found; texts: %q", tester.Texts())
	}
	rowCenter := status.Y + status.H/2
	for name, button := range map[string]ui.Rect{"新建": newButton, "钉住": pinButton, "删除": deleteButton} {
		if delta := button.Y + button.H/2 - rowCenter; delta < -4 || delta > 4 {
			t.Fatalf("%s is on another row: status %#v, button %#v", name, status, button)
		}
	}
	if status.X+status.W >= newButton.X {
		t.Fatalf("status %#v overlaps the action row starting at %.1f", status, newButton.X)
	}
	if newButton.X >= pinButton.X || pinButton.X >= deleteButton.X {
		t.Fatalf("action order is %v, %v, %v", newButton, pinButton, deleteButton)
	}

	icon, ok := tester.Find("快捷键信息")
	if !ok {
		t.Fatalf("info icon not found; texts: %q", tester.Texts())
	}
	statusInk, ok := renderedInkRows(tester.Image(), status)
	if !ok {
		t.Fatal("status text has no ink")
	}
	iconInk, ok := renderedInkRows(tester.Image(), icon)
	if !ok {
		t.Fatal("info icon has no ink")
	}
	statusInkCenter := float64(statusInk[0]+statusInk[1]) / 2
	iconInkCenter := float64(iconInk[0]+iconInk[1]) / 2
	if diff := iconInkCenter - statusInkCenter; diff < -1 || diff > 1 {
		t.Fatalf("info icon ink center %.1f, status ink center %.1f, difference %.1f", iconInkCenter, statusInkCenter, diff)
	}
	if iconHeight, statusHeight := iconInk[1]-iconInk[0], statusInk[1]-statusInk[0]; iconHeight > statusHeight+2 {
		t.Fatalf("info icon ink is %dpx tall against %dpx of text", iconHeight, statusHeight)
	}

	if output := os.Getenv("MOTE_FOOTER_SCREENSHOT"); output != "" {
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

func TestNotesFooterFitsNarrowWindow(t *testing.T) {
	updated := time.Date(2026, 10, 16, 14, 27, 0, 0, time.Local)
	s := newApp(&store{path: "/Users/test/Library/Application Support/Mote/notes.db"},
		note{ID: 4, Markdown: "# 周五出发", UpdatedAt: updated},
		[]noteSummary{{ID: 4, Title: "周五出发", UpdatedAt: updated}},
		defaultTimerState(),
		0,
	)
	for _, width := range []int{400, 460} {
		tester := ui.NewTester(s.view, width, 440)
		deleteButton, ok := tester.Find("删除")
		if !ok {
			t.Fatalf("width %d: delete button not found", width)
		}
		if right := deleteButton.X + deleteButton.W; right > float32(width) {
			t.Fatalf("width %d: delete button overflows at %.1f", width, right)
		}
		status, ok := tester.Find("已保存 · 14:27")
		if !ok {
			t.Fatalf("width %d: status not found", width)
		}
		if status.X+status.W > deleteButton.X {
			t.Fatalf("width %d: status overlaps the actions", width)
		}
	}
}

func TestDeleteButtonAsksBeforeRemoving(t *testing.T) {
	st, err := openMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	saved, err := st.Save(context.Background(), 0, "# 待删除")
	if err != nil {
		t.Fatal(err)
	}
	s := newApp(st, saved, nil, defaultTimerState(), 0)
	tester := ui.NewTester(s.view, 460, 540)

	if err := tester.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("删除这条记录？") {
		t.Fatalf("confirm dialog did not open; texts: %q", tester.Texts())
	}
	if output := os.Getenv("MOTE_DIALOG_SCREENSHOT"); output != "" {
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
	// Cancelling keeps the record.
	if err := tester.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if s.currentID != saved.ID {
		t.Fatalf("cancel removed the record: currentID %d", s.currentID)
	}

	// Escape dismisses the dialog without deleting.
	if err := tester.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tester.Key(0, ui.KeyEscape)
	if s.currentID != saved.ID {
		t.Fatalf("escape removed the record: currentID %d", s.currentID)
	}

	if err := tester.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("删除记录"); err != nil {
		t.Fatal(err)
	}
	if s.currentID != 0 || s.draft != "" {
		t.Fatalf("confirm did not remove the record: %#v", s)
	}
	recent, err := st.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 0 {
		t.Fatalf("records left after delete: %#v", recent)
	}
}

func TestPinTogglesAndUnpinClearsIt(t *testing.T) {
	s := newApp(&store{path: ":memory:"}, note{ID: 1, Markdown: "# 钉住"}, nil, defaultTimerState(), 0)
	tester := ui.NewTester(s.view, 460, 540)

	if s.pinned {
		t.Fatal("pinned before anything was clicked")
	}
	if err := tester.Click("钉住"); err != nil {
		t.Fatal(err)
	}
	if !s.pinned {
		t.Fatal("clicking 钉住 did not turn the pin on")
	}
	if err := tester.Click("钉住"); err != nil {
		t.Fatal(err)
	}
	if s.pinned {
		t.Fatal("clicking 钉住 again did not turn the pin off")
	}

	// unpin runs from the pinned window and from the panel, so it must cope
	// with the window not existing yet.
	s.pinned = true
	s.unpin()
	if s.pinned {
		t.Fatal("unpin left the pin on")
	}
}

func TestPinnedWindowShowsTheDraftAndEditsIt(t *testing.T) {
	draft := "# 钉住\n\n同一份草稿"
	s := newApp(&store{path: ":memory:"}, note{ID: 2, Markdown: draft}, nil, defaultTimerState(), 0)
	tester := ui.NewTester(s.pinnedView, 420, 480)

	for _, want := range []string{"编辑", "预览", "取消钉住", "已保存", "自动保存"} {
		if !tester.HasText(want) {
			t.Errorf("pinned window missing %q; texts: %q", want, tester.Texts())
		}
	}
	if err := tester.Click("预览"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("同一份草稿") {
		t.Fatalf("preview does not show the draft; texts: %q", tester.Texts())
	}
	if err := tester.Click("编辑"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Markdown 内容"); err != nil {
		t.Fatal(err)
	}
	tester.Type("补一句")
	if s.autoSaveTimer != nil {
		s.autoSaveTimer.Stop()
	}
	if s.draft == draft {
		t.Fatalf("typing in the pinned window did not reach the shared draft: %q", s.draft)
	}
	if s.phase != phaseDirty {
		t.Fatalf("phase %v after typing, want dirty so autosave runs", s.phase)
	}

	if output := os.Getenv("MOTE_PINNED_SCREENSHOT"); output != "" {
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
