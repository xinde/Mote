package main

import (
	"errors"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestUpdateHitTargetOnlyCoversMoteTitle(t *testing.T) {
	s := newApp(&store{path: ":memory:"}, note{}, nil, defaultTimerState(), 0)
	tester := ui.NewTester(s.view, 460, 540)

	target, ok := tester.Find("检查 Mote 更新")
	if !ok {
		t.Fatalf("update target is missing: %q", tester.Texts())
	}
	if target.W > 80 || target.H > 32 {
		t.Fatalf("update target is larger than the Mote title: %#v", target)
	}
	detail, ok := tester.Find("记录想法，也留一段专注时间")
	if !ok {
		t.Fatalf("header detail is missing: %q", tester.Texts())
	}
	if target.Y+target.H > detail.Y {
		t.Fatalf("update target %#v overlaps non-clickable detail %#v", target, detail)
	}
}

func TestUpdateProgressAppearsInHeader(t *testing.T) {
	s := newApp(&store{path: "/tmp/mote-test.db"}, note{}, nil, defaultTimerState(), 0)
	s.updatePhase = updateChecking

	tester := ui.NewTester(s.view, 460, 540)
	if !tester.HasText("正在检查 GitHub 更新") || !tester.HasText("正在更新 Mote") {
		t.Fatalf("checking state missing from header: %q", tester.Texts())
	}

	s.updatePhase = updateInstalling
	s.updateVersion = "0.7.0"
	s.updateProgress = 42
	tester.Frame()
	if !tester.HasText("正在升级到 Mote 0.7.0 · 42%") {
		t.Fatalf("install progress missing from header: %q", tester.Texts())
	}

	tester.SetSize(375, 440)
	if !tester.HasText("正在升级到 Mote 0.7.0 · 42%") || !tester.HasText("记录") || !tester.HasText("专注") {
		t.Fatalf("compact update header is incomplete: %q", tester.Texts())
	}
	if output := os.Getenv("MOTE_UPDATE_SCREENSHOT"); output != "" {
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

func TestPinnedWindowShowsUpdateProgress(t *testing.T) {
	s := newApp(&store{path: ":memory:"}, note{}, nil, defaultTimerState(), 0)
	s.updatePhase = updateInstalling
	s.updateVersion = "0.7.0"
	s.updateProgress = 42

	tester := ui.NewTester(s.pinnedView, 420, 480)
	if !tester.HasText("正在升级到 Mote 0.7.0 · 42%") || !tester.HasText("正在更新 Mote") {
		t.Fatalf("pinned update progress is missing: %q", tester.Texts())
	}
}

func TestSaveFailureCancelsUpdateRelaunch(t *testing.T) {
	s := newApp(&store{path: ":memory:"}, note{ID: 1, Markdown: "旧内容"}, nil, defaultTimerState(), 0)
	s.draft = "尚未保存的新内容"
	s.phase = phaseSaving
	s.quitAfterSave = true
	s.relaunchAfterSave = true
	s.updatePhase = updateInstalling

	s.finishSave(1, s.draft, note{}, nil, errors.New("disk full"))

	if s.quitAfterSave || s.relaunchAfterSave || s.quitting {
		t.Fatalf("failed save left relaunch armed: %#v", s)
	}
	if s.updatePhase != updateIdle || !s.hasUnsavedDraft() {
		t.Fatalf("failed save lost retry state: phase=%v update=%v", s.phase, s.updatePhase)
	}
	if s.toast == "" {
		t.Fatal("failed save did not explain why relaunch was cancelled")
	}
}
