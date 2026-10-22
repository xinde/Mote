package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"testing"
	"time"
)

func TestTimerFormattingAndProgress(t *testing.T) {
	if got := formatTimer(1500); got != "25:00" {
		t.Fatalf("formatTimer(1500) = %q", got)
	}
	now := time.Date(2026, 10, 16, 10, 0, 0, 0, time.Local)
	state := timerState{
		Phase:         timerFocus,
		Status:        timerActive,
		DurationSecs:  1500,
		RemainingSecs: 1500,
		EndsAt:        now.Add(750 * time.Second),
	}
	if got := state.remaining(now); got != 750 {
		t.Fatalf("remaining = %d, want 750", got)
	}
	if got := state.progress(now); got != 0.5 {
		t.Fatalf("progress = %v, want 0.5", got)
	}
}

func TestTimerTrayIconKeepsFixedWidth(t *testing.T) {
	first := timerTrayIcon(14*60 + 31)
	second := timerTrayIcon(8*60 + 8)
	for name, data := range map[string][]byte{"14:31": first, "08:08": second} {
		image, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("decode %s icon: %v", name, err)
		}
		if image.Bounds().Dx() != 104 || image.Bounds().Dy() != 32 {
			t.Fatalf("%s icon size %v, want 104x32", name, image.Bounds())
		}
	}
	if bytes.Equal(first, second) {
		t.Fatal("different times rendered identical tray icons")
	}
}

func TestTimerTrayDigitsAlignWithIcon(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(timerTrayIcon(14*60 + 31)))
	if err != nil {
		t.Fatal(err)
	}
	iconTop, iconBottom, ok := inkRows(img, 0, 28)
	if !ok {
		t.Fatal("note icon has no ink")
	}
	digitTop, digitBottom, ok := inkRows(img, 30, img.Bounds().Dx())
	if !ok {
		t.Fatal("timer digits have no ink")
	}
	iconCenter := float64(iconTop+iconBottom) / 2
	digitCenter := float64(digitTop+digitBottom) / 2
	if diff := digitCenter - iconCenter; diff < -1 || diff > 1 {
		t.Fatalf("digits center %.1f, icon center %.1f, difference %.1f", digitCenter, iconCenter, diff)
	}
	if output := os.Getenv("MOTE_TRAY_SCREENSHOT"); output != "" {
		if err := os.WriteFile(output, timerTrayIcon(23*60+51), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// inkRows returns the first and last row holding ink between x0 and x1.
func inkRows(img image.Image, x0, x1 int) (int, int, bool) {
	top, bottom := -1, -1
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := x0; x < x1; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				if top < 0 {
					top = y
				}
				bottom = y
				break
			}
		}
	}
	return top, bottom, top >= 0
}

func TestTimerPauseResumeAndReset(t *testing.T) {
	st, err := openMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s := newApp(st, note{}, nil, defaultTimerState(), 0)
	s.startTimer()
	if s.timer.Status != timerActive || s.timer.EndsAt.IsZero() {
		t.Fatalf("started timer %#v", s.timer)
	}
	s.pauseTimer()
	if s.timer.Status != timerPaused || s.timer.RemainingSecs <= 0 || !s.timer.EndsAt.IsZero() {
		t.Fatalf("paused timer %#v", s.timer)
	}
	s.resumeTimer()
	if s.timer.Status != timerActive || s.timer.EndsAt.IsZero() {
		t.Fatalf("resumed timer %#v", s.timer)
	}
	s.resetTimer()
	if s.timer.Status != timerIdle || s.timer.RemainingSecs != 25*60 {
		t.Fatalf("reset timer %#v", s.timer)
	}
}

func TestTimerStateAndCompletionsPersist(t *testing.T) {
	st, err := openMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	state := defaultTimerState()
	state.FocusMinutes = 42
	state.DurationSecs = 42 * 60
	state.RemainingSecs = 137
	state.Status = timerPaused
	if err := st.SaveTimerState(ctx, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.LoadTimerState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FocusMinutes != 42 || loaded.RemainingSecs != 137 || loaded.Status != timerPaused {
		t.Fatalf("loaded timer %#v", loaded)
	}

	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.Local)
	if err := st.RecordTimerCompletion(ctx, timerFocus, 25*60, now); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordTimerCompletion(ctx, timerBreak, 5*60, now); err != nil {
		t.Fatal(err)
	}
	count, err := st.TodayPomodoros(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("today focus count %d, want 1", count)
	}
}
