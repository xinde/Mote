package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// alertScreen fills the screen when a focus session ends. It is drawn in the
// window Mote already has open, which is stretched over the display for it,
// so no second window is created.
func (s *app) alertScreen(c *ui.Context) {
	c.SetTheme(warmTheme(c.Theme().Dark))
	t := c.Theme()

	if c.Shortcut(0, ui.KeyEscape) {
		s.hideTimerAlert()
	}

	// A translucent veil: whatever was on screen stays readable behind the
	// prompt, blurred where the system gives the window a material.
	c.Root().Background(ui.Transparent)
	backdrop := t.Background.Alpha(0.62)

	ui.Column(c).Fill().Background(backdrop).Center().Padding(48).Gap(18).Children(func() {
		ui.Text(c, "专注完成").
			Font("New York").
			FontSize(84).
			Bold()
		ui.Text(c, "这段时间结束了，休息一下再继续。").
			FontSize(19).
			TextColor(t.TextMuted)
		ui.Textf(c, "今天已完成 %d 个番茄", s.todayPomodoros).
			FontSize(14).
			TextColor(t.TextMuted)

		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "稍后再说").FontSize(15).Padding(8, 20).Clicked() {
				s.hideTimerAlert()
			}
			if ui.PrimaryButton(c, "开始休息").FontSize(15).Padding(8, 20).Clicked() {
				s.hideTimerAlert()
				s.startTimer()
			}
		})
	})
}

// showTimerAlert stretches the window the user is looking at over its
// display and switches it to the alert, remembering what to put back.
func (s *app) showTimerAlert() {
	win := s.win
	if s.pinWin != nil && s.pinWin.IsVisible() {
		win = s.pinWin
	}
	if win == nil {
		return
	}

	s.alertWindow = win
	s.alertRestore = win.Bounds()
	s.alertWasHidden = !win.IsVisible()

	area := mygo.Screen.DisplayNearestPoint(mygo.Point{
		X: s.alertRestore.X + 1,
		Y: s.alertRestore.Y + 1,
	}).Bounds
	win.SetBounds(area)
	win.Show()
	win.Focus()

	s.alertOpen = true
	s.refresh()
}

func visibleWindowBounds(bounds mygo.Rectangle) mygo.Rectangle {
	area := mygo.Screen.DisplayNearestPoint(mygo.Point{
		X: bounds.X + bounds.Width/2,
		Y: bounds.Y + bounds.Height/2,
	}).WorkArea
	bounds.Width = min(bounds.Width, area.Width)
	bounds.Height = min(bounds.Height, area.Height)
	bounds.X = max(area.X, min(bounds.X, area.X+area.Width-bounds.Width))
	bounds.Y = max(area.Y, min(bounds.Y, area.Y+area.Height-bounds.Height))
	return bounds
}

// hideTimerAlert puts the window back where it was, or hides it when the
// alert was the only reason it showed.
func (s *app) hideTimerAlert() {
	s.alertOpen = false

	if win := s.alertWindow; win != nil {
		// Always put the size back: the next show only moves the panel, so a
		// window left screen-sized would stay that way. Clamp stale coordinates
		// when a display was disconnected while the alert was open.
		win.SetBounds(visibleWindowBounds(s.alertRestore))
		if s.alertWasHidden {
			win.Hide()
		}
		s.alertWindow = nil
	}
	s.refresh()
}
