package main

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"
)

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
