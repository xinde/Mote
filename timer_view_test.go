package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTimerPhaseTabsAreCentered(t *testing.T) {
	selected := 0
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Tabs(c.Key("timer-phase"), &selected, "专注", "休息").
				AlignSelf(ui.Center).
				Label("计时阶段")
		})
	}, 400, 120)
	focus, ok := tester.Find("专注")
	if !ok {
		t.Fatalf("focus tab not found; texts: %q", tester.Texts())
	}
	pause, ok := tester.Find("休息")
	if !ok {
		t.Fatalf("break tab not found; texts: %q", tester.Texts())
	}
	left := min(focus.X, pause.X)
	right := max(focus.X+focus.W, pause.X+pause.W)
	if center := (left + right) / 2; center < 199 || center > 201 {
		t.Fatalf("visible timer tabs center %.1f, want window center 200; focus %#v break %#v", center, focus, pause)
	}
}

func TestTimerRunningView(t *testing.T) {
	state := defaultTimerState()
	state.Status = timerPaused
	state.DurationSecs = 25 * 60
	state.RemainingSecs = 12*60 + 30
	s := newApp(&store{path: "/Users/test/Library/Application Support/Mote/notes.db"}, note{}, nil, state, 3)
	s.section = 1

	tester := ui.NewTester(s.view, 400, 440)
	for _, text := range []string{"12:30", "已暂停", "今天完成 3 个番茄", "继续", "重置"} {
		if !tester.HasText(text) {
			t.Errorf("running timer view missing %q; texts: %q", text, tester.Texts())
		}
	}

	if output := os.Getenv("MOTE_TIMER_SCREENSHOT"); output != "" {
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
