package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"
)

const (
	timerFocus = "focus"
	timerBreak = "break"

	timerIdle   = "idle"
	timerActive = "running"
	timerPaused = "paused"
)

type timerState struct {
	Phase         string    `json:"phase"`
	Status        string    `json:"status"`
	FocusMinutes  int       `json:"focusMinutes"`
	BreakMinutes  int       `json:"breakMinutes"`
	RemainingSecs int       `json:"remainingSeconds"`
	DurationSecs  int       `json:"durationSeconds"`
	EndsAt        time.Time `json:"endsAt,omitempty"`
	// Alert asks for a prompt over the window when a focus session ends. Off
	// by default: the notification and the chime are enough.
	Alert bool `json:"alert,omitempty"`
}

func defaultTimerState() timerState {
	return timerState{
		Phase:         timerFocus,
		Status:        timerIdle,
		FocusMinutes:  25,
		BreakMinutes:  5,
		RemainingSecs: 25 * 60,
		DurationSecs:  25 * 60,
	}
}

func normalizeTimerState(t timerState) timerState {
	if t.FocusMinutes < 1 || t.FocusMinutes > 120 {
		t.FocusMinutes = 25
	}
	if t.BreakMinutes < 1 || t.BreakMinutes > 60 {
		t.BreakMinutes = 5
	}
	if t.Phase != timerFocus && t.Phase != timerBreak {
		t.Phase = timerFocus
	}
	if t.Status != timerIdle && t.Status != timerActive && t.Status != timerPaused {
		t.Status = timerIdle
	}
	if t.DurationSecs <= 0 {
		t.DurationSecs = t.minutesForPhase() * 60
	}
	if t.RemainingSecs <= 0 && t.Status != timerActive {
		t.RemainingSecs = t.DurationSecs
	}
	return t
}

func (t timerState) minutesForPhase() int {
	if t.Phase == timerBreak {
		return t.BreakMinutes
	}
	return t.FocusMinutes
}

func (t timerState) remaining(now time.Time) int {
	if t.Status != timerActive {
		return max(0, t.RemainingSecs)
	}
	remaining := int(math.Ceil(t.EndsAt.Sub(now).Seconds()))
	return max(0, remaining)
}

func (t timerState) progress(now time.Time) float64 {
	if t.DurationSecs <= 0 {
		return 0
	}
	return min(1, max(0, 1-float64(t.remaining(now))/float64(t.DurationSecs)))
}

func formatTimer(seconds int) string {
	seconds = max(0, seconds)
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func (s *app) timerView(c *ui.Context) {
	theme := c.Theme()
	now := c.Now()
	if s.timer.Status == timerActive {
		c.After(time.Second)
		if !now.Before(s.timer.EndsAt) {
			s.completeTimer()
			now = c.Now()
		}
	}

	ui.Column(c).Grow(1).FillWidth().Padding(4, 10, 8).Gap(18).Children(func() {
		phase := 0
		if s.timer.Phase == timerBreak {
			phase = 1
		}
		if ui.Tabs(c.Key("timer-phase"), &phase, "专注", "休息").
			AlignSelf(ui.Center).
			Label("计时阶段").
			Disabled(s.timer.Status != timerIdle).
			Changed() {
			if phase == 1 {
				s.timer.Phase = timerBreak
			} else {
				s.timer.Phase = timerFocus
			}
			s.resetTimerDuration()
			s.persistTimer()
		}

		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			phaseLabel := "准备专注"
			if s.timer.Phase == timerBreak {
				phaseLabel = "休息一下"
			}
			if s.timer.Status == timerActive {
				phaseLabel = "正在" + map[string]string{timerFocus: "专注", timerBreak: "休息"}[s.timer.Phase]
			} else if s.timer.Status == timerPaused {
				phaseLabel = "已暂停"
			}
			ui.Text(c, phaseLabel).FontSize(12).TextColor(theme.TextMuted)
			ui.Text(c, formatTimer(s.timer.remaining(now))).
				Font("Menlo").
				FontSize(64).
				FontWeight(500).
				LetterSpacing(-1.2)
			ui.Progress(c, s.timer.progress(now)).
				Size(300, 4).
				Label("当前计时进度")
			ui.Textf(c, "今天完成 %d 个番茄", s.todayPomodoros).
				FontSize(11).
				TextColor(theme.TextMuted)
		})

		if s.timer.Status == timerIdle {
			ui.Column(c).AlignSelf(ui.Center).Width(240).Gap(8).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Text(c, "专注时长").Grow(1).FontSize(11).TextColor(theme.TextMuted)
					focus := float64(s.timer.FocusMinutes)
					if ui.NumberInput(c.Key("focus-minutes"), &focus, 1, 120, 1).
						Width(128).
						Label("专注分钟").
						Changed() {
						s.timer.FocusMinutes = int(focus)
						s.resetTimerDuration()
						s.persistTimer()
					}
				})
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Text(c, "休息时长").Grow(1).FontSize(11).TextColor(theme.TextMuted)
					pause := float64(s.timer.BreakMinutes)
					if ui.NumberInput(c.Key("break-minutes"), &pause, 1, 60, 1).
						Width(128).
						Label("休息分钟").
						Changed() {
						s.timer.BreakMinutes = int(pause)
						s.resetTimerDuration()
						s.persistTimer()
					}
				})
			})
		}

		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8).Children(func() {
			if ui.Switch(c.Key("alert"), &s.timer.Alert).
				Label("完成提示").
				Tooltip("专注结束时在窗口上弹出提示，而不只是发通知").
				Changed() {
				s.persistTimer()
			}
			ui.Text(c, "专注结束时弹窗提示").
				FontSize(11).
				TextColor(theme.TextMuted)
			ui.Spacer(c)
			if s.timer.Status != timerIdle {
				if ui.Button(c, "重置").Clicked() {
					s.resetTimer()
				}
			}
			label := "开始专注"
			if s.timer.Phase == timerBreak {
				label = "开始休息"
			}
			if s.timer.Status == timerActive {
				label = "暂停"
			} else if s.timer.Status == timerPaused {
				label = "继续"
			}
			if ui.PrimaryButton(c, label).MinWidth(112).Clicked() {
				s.toggleTimer()
			}
		})
	})
}

func (s *app) toggleTimer() {
	switch s.timer.Status {
	case timerActive:
		s.pauseTimer()
	case timerPaused:
		s.resumeTimer()
	default:
		s.startTimer()
	}
}

func (s *app) startTimer() {
	s.timer.DurationSecs = s.timer.minutesForPhase() * 60
	s.timer.RemainingSecs = s.timer.DurationSecs
	s.timer.Status = timerActive
	s.timer.EndsAt = time.Now().Add(time.Duration(s.timer.RemainingSecs) * time.Second)
	s.scheduleTimer()
	s.persistTimer()
}

func (s *app) pauseTimer() {
	s.timer.RemainingSecs = s.timer.remaining(time.Now())
	s.timer.Status = timerPaused
	s.timer.EndsAt = time.Time{}
	s.stopTimerSchedule()
	s.persistTimer()
}

func (s *app) resumeTimer() {
	if s.timer.RemainingSecs <= 0 {
		s.timer.RemainingSecs = s.timer.minutesForPhase() * 60
	}
	s.timer.Status = timerActive
	s.timer.EndsAt = time.Now().Add(time.Duration(s.timer.RemainingSecs) * time.Second)
	s.scheduleTimer()
	s.persistTimer()
}

func (s *app) resetTimer() {
	s.stopTimerSchedule()
	s.timer.Status = timerIdle
	s.timer.EndsAt = time.Time{}
	s.resetTimerDuration()
	s.persistTimer()
}

func (s *app) resetTimerDuration() {
	s.timer.DurationSecs = s.timer.minutesForPhase() * 60
	s.timer.RemainingSecs = s.timer.DurationSecs
}

func (s *app) scheduleTimer() {
	s.stopTimerSchedule()
	delay := time.Until(s.timer.EndsAt)
	if delay < 0 {
		delay = 0
	}
	s.timerDeadline = time.AfterFunc(delay, func() {
		s.update(s.completeTimer)
	})
	s.startTimerTicker()
}

func (s *app) startTimerTicker() {
	if s.timerTickerStop != nil {
		close(s.timerTickerStop)
	}
	stop := make(chan struct{})
	endsAt := s.timer.EndsAt
	s.timerTickerStop = stop
	if s.tray != nil {
		if err := s.tray.SetIcon(timerTrayIcon(s.timer.remaining(time.Now())), true); err != nil {
			log.Println("set timer tray icon:", err)
		}
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				remaining := max(0, int(math.Ceil(endsAt.Sub(now).Seconds())))
				if s.tray != nil {
					if err := s.tray.SetIcon(timerTrayIcon(remaining), true); err != nil {
						log.Println("set timer tray icon:", err)
					}
				}
			case <-stop:
				return
			}
		}
	}()
}

func (s *app) stopTimerSchedule() {
	if s.timerDeadline != nil {
		s.timerDeadline.Stop()
		s.timerDeadline = nil
	}
	if s.timerTickerStop != nil {
		close(s.timerTickerStop)
		s.timerTickerStop = nil
	}
	if s.tray != nil {
		s.tray.SetTitle("")
		if err := s.tray.SetIcon(trayIcon(), true); err != nil {
			log.Println("restore tray icon:", err)
		}
	}
}

func timerTrayIcon(seconds int) []byte {
	const (
		width  = 104
		height = 32
	)
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	ink := color.NRGBA{A: 255}
	for _, rect := range []image.Rectangle{
		image.Rect(3, 3, 25, 6), image.Rect(3, 26, 25, 29),
		image.Rect(3, 6, 6, 26), image.Rect(22, 6, 25, 26),
		image.Rect(10, 10, 19, 12), image.Rect(10, 16, 19, 18), image.Rect(10, 22, 16, 24),
	} {
		draw.Draw(img, rect, image.NewUniform(ink), image.Point{}, draw.Src)
	}
	drawTimerDigits(img, formatTimer(seconds), 32, ink)
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

var timerGlyphs = map[byte][5]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "001", "001", "001"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
	':': {"000", "010", "000", "010", "000"},
}

func drawTimerDigits(img draw.Image, text string, x int, ink color.Color) {
	const (
		scale   = 3
		rows    = 5
		advance = 12
	)
	// Center the glyphs on the icon's optical center instead of a fixed
	// offset, so the digits and the note icon share one vertical center.
	y := img.Bounds().Min.Y + (img.Bounds().Dy()-rows*scale)/2
	for _, ch := range []byte(text) {
		glyph, ok := timerGlyphs[ch]
		if !ok {
			x += advance
			continue
		}
		for row, line := range glyph {
			for col, pixel := range []byte(line) {
				if pixel != '1' {
					continue
				}
				rect := image.Rect(x+col*scale, y+row*scale, x+(col+1)*scale, y+(row+1)*scale)
				draw.Draw(img, rect, image.NewUniform(ink), image.Point{}, draw.Src)
			}
		}
		x += advance
	}
}

func (s *app) completeTimer() {
	if s.timer.Status != timerActive || time.Now().Before(s.timer.EndsAt) {
		return
	}
	completedPhase := s.timer.Phase
	s.finishTimerPhase(completedPhase, s.timer.DurationSecs)

	s.notifyTimerComplete(completedPhase)
	if s.alertWanted(completedPhase) {
		s.showTimerAlert()
	}
}

// finishTimerPhase moves the timer to the other phase, records the session
// that ended and saves the state, without telling the user about it.
func (s *app) finishTimerPhase(completedPhase string, completedDuration int) {
	s.stopTimerSchedule()
	if completedPhase == timerFocus {
		s.todayPomodoros++
		s.timer.Phase = timerBreak
	} else {
		s.timer.Phase = timerFocus
	}
	s.timer.Status = timerIdle
	s.timer.EndsAt = time.Time{}
	s.resetTimerDuration()
	s.persistTimer()

	if err := s.store.RecordTimerCompletion(context.Background(), completedPhase, completedDuration, time.Now()); err != nil {
		log.Println("record timer completion:", err)
	}
	s.refresh()
}

// fullScreenAlertWanted reports whether the phase that ended asks for the
// full-screen prompt: only a focus session, and only when it is switched on.
func (s *app) alertWanted(completedPhase string) bool {
	return completedPhase == timerFocus && s.timer.Alert
}

func (s *app) notifyTimerComplete(phase string) {
	title, body := "专注完成", "休息一下，下一段会更清醒。"
	if phase == timerBreak {
		title, body = "休息结束", "可以开始下一段专注了。"
	}
	if s.chime != nil {
		s.chime.NewPlayer().SetVolume(0.35).Play()
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID:     "mote-timer",
		Title:  title,
		Body:   body,
		Silent: true,
		Group:  "timer",
	})
	n.OnClick(func() {
		s.section = 1
		s.show()
	})
	go func() {
		if err := n.Show(); err != nil {
			log.Println("show timer notification:", err)
		}
	}()
}

func (s *app) persistTimer() {
	if err := s.store.SaveTimerState(context.Background(), s.timer); err != nil {
		log.Println("save timer state:", err)
	}
}

func makeTimerChime() *audio.Sound {
	const sampleRate = 48000
	const seconds = 0.32
	samples := make([]float32, int(sampleRate*seconds))
	for i := range samples {
		t := float64(i) / sampleRate
		frequency := 660.0
		if t > 0.14 {
			frequency = 880
		}
		envelope := math.Sin(math.Pi * min(1, t/seconds))
		samples[i] = float32(0.09 * envelope * math.Sin(2*math.Pi*frequency*t))
	}
	return audio.NewSound(sampleRate, 1, samples)
}
