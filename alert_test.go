package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAlertScreen(t *testing.T) {
	s := newApp(&store{path: ":memory:"}, note{}, nil, defaultTimerState(), 3)
	tester := ui.NewTester(s.alertScreen, 900, 600)

	for _, want := range []string{"专注完成", "这段时间结束了，休息一下再继续。", "今天已完成 3 个番茄", "开始休息", "稍后再说"} {
		if !tester.HasText(want) {
			t.Errorf("alert missing %q; texts: %q", want, tester.Texts())
		}
	}

	if output := os.Getenv("MOTE_ALERT_SCREENSHOT"); output != "" {
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

func TestAlertTakesOverTheWindow(t *testing.T) {
	s := newApp(&store{path: ":memory:"}, note{ID: 1, Markdown: "# 记录"}, nil, defaultTimerState(), 2)
	tester := ui.NewTester(s.view, 900, 600)

	if tester.HasText("专注完成") {
		t.Fatal("the alert shows while it is closed")
	}
	s.alertOpen = true
	tester.Frame()
	if !tester.HasText("专注完成") || !tester.HasText("开始休息") {
		t.Fatalf("the alert is missing; texts: %q", tester.Texts())
	}
	if tester.HasText("已保存") {
		t.Fatalf("the note view still shows under the alert; texts: %q", tester.Texts())
	}

	// Closing it without a window to restore is harmless.
	s.hideTimerAlert()
	if s.alertOpen {
		t.Fatal("hideTimerAlert left the alert open")
	}
}

func TestAlertBackdropIsTranslucent(t *testing.T) {
	s := newApp(&store{path: ":memory:"}, note{}, nil, defaultTimerState(), 0)
	tester := ui.NewTester(s.alertScreen, 400, 300)

	_, _, _, a := tester.Image().At(3, 3).RGBA()
	if a >= 0xffff {
		t.Fatal("the alert backdrop is opaque, so nothing shows through")
	}
	if a < 0x8000 {
		t.Fatalf("the alert backdrop is too see-through to read on: alpha %d", a>>8)
	}
}
