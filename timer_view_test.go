package main

import (
	"image/png"
	"os"
	"testing"
	"time"

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

func TestActiveTimerFooterPlacesResetAndPauseOnRight(t *testing.T) {
	state := defaultTimerState()
	state.Status = timerActive
	state.EndsAt = time.Now().Add(12 * time.Minute)
	s := newApp(&store{path: ":memory:"}, note{}, nil, state, 0)
	s.section = 1
	tester := ui.NewTester(s.view, 400, 440)

	hint, ok := tester.Find("专注结束时弹窗提示")
	if !ok {
		t.Fatalf("alert hint missing; texts: %q", tester.Texts())
	}
	reset, ok := tester.Find("重置")
	if !ok {
		t.Fatalf("reset button missing; texts: %q", tester.Texts())
	}
	pause, ok := tester.Find("暂停")
	if !ok {
		t.Fatalf("pause button missing; texts: %q", tester.Texts())
	}
	if !(hint.X+hint.W < reset.X && reset.X+reset.W < pause.X && pause.X+pause.W <= 400) {
		t.Fatalf("alert, reset and pause overlap or are out of order: %#v %#v %#v", hint, reset, pause)
	}
	if delta := reset.Y + reset.H/2 - (pause.Y + pause.H/2); delta < -4 || delta > 4 {
		t.Fatalf("reset and pause are on different rows: %#v %#v", reset, pause)
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

	for _, width := range []int{400, 460} {
		tester.SetSize(width, 440)
		switchControl, ok := tester.Find("完成提示")
		if !ok {
			t.Fatalf("width %d: alert switch missing", width)
		}
		hint, ok := tester.Find("专注结束时弹窗提示")
		if !ok {
			t.Fatalf("width %d: alert hint missing", width)
		}
		reset, ok := tester.Find("重置")
		if !ok {
			t.Fatalf("width %d: reset button missing", width)
		}
		resume, ok := tester.Find("继续")
		if !ok {
			t.Fatalf("width %d: resume button missing", width)
		}
		if !(switchControl.X < hint.X && hint.X+hint.W < reset.X && reset.X+reset.W < resume.X && resume.X+resume.W <= float32(width)) {
			t.Fatalf("width %d: controls overlap or are out of order: switch=%#v hint=%#v reset=%#v resume=%#v", width, switchControl, hint, reset, resume)
		}
		center := resume.Y + resume.H/2
		for name, box := range map[string]ui.Rect{"开关": switchControl, "说明": hint, "重置": reset} {
			if delta := box.Y + box.H/2 - center; delta < -4 || delta > 4 {
				t.Fatalf("width %d: %s is not in the action row: %#v vs %#v", width, name, box, resume)
			}
		}
	}
	tester.SetSize(400, 440)

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
