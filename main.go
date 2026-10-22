package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

const (
	appName      = "Mote"
	globalHotkey = "CmdOrCtrl+Shift+M"
	autoSaveWait = 700 * time.Millisecond
)

// Buttons of the delete confirmation, in the order AlertDialog shows them.
// Keeping the indexes named stops a reorder from silently deleting on cancel.
const (
	deleteDialogDelete = iota
	deleteDialogCancel
)

var deleteDialogButtons = [2]string{"删除记录", "取消"}

const (
	deleteDialogTitle   = "删除这条记录？"
	deleteDialogMessage = "记录会从数据库移除，无法撤销。"
)

var infoIcon = ui.MustParseSVG([]byte(`<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="10" cy="10" r="7.2"/><path d="M10 8.7v5"/><circle cx="10" cy="6" r=".8" fill="currentColor" stroke="none"/></svg>`))

type savePhase int

const (
	phaseIdle savePhase = iota
	phaseDirty
	phaseSaving
	phaseSaved
	phaseError
)

type app struct {
	win    *mygo.Window
	tray   *mygo.Tray
	store  *store
	editor ui.Handle
	md     markdown.Options
	chime  *audio.Sound

	section         int
	timer           timerState
	todayPomodoros  int
	timerDeadline   *time.Timer
	timerTickerStop chan struct{}

	currentID         int64
	draft             string
	savedText         string
	lastSavedAt       time.Time
	recent            []noteSummary
	recentChoice      string
	viewMode          int
	phase             savePhase
	statusDetail      string
	revision          uint64
	savePending       bool
	quitAfterSave     bool
	relaunchAfterSave bool
	confirmDelete     bool
	pinned            bool
	pinWin            *mygo.Window
	pinEditor         ui.Handle
	pinMode           int
	pinPlaced         bool
	alertOpen         bool
	alertWindow       *mygo.Window
	alertRestore      mygo.Rectangle
	alertWasHidden    bool
	autoSaveTimer     *time.Timer
	quitting          bool
	updatePhase       updatePhase
	updateVersion     string
	updateProgress    int
	toast             string
}

func newApp(st *store, current note, recent []noteSummary, timer timerState, todayPomodoros int) *app {
	s := &app{
		store:          st,
		timer:          normalizeTimerState(timer),
		todayPomodoros: todayPomodoros,
		chime:          makeTimerChime(),
		currentID:      current.ID,
		draft:          current.Markdown,
		savedText:      current.Markdown,
		lastSavedAt:    current.UpdatedAt,
		recent:         recent,
		md: markdown.Options{
			WrapCode: true,
			Theme: &markdown.Theme{
				FontSize:        14,
				LineHeight:      1.65,
				CodeFont:        "Menlo",
				BlockSpacing:    0.9,
				CodeBlockRadius: 8,
			},
		},
	}
	if current.ID == 0 && strings.TrimSpace(current.Markdown) == "" {
		s.phase = phaseIdle
	} else {
		s.phase = phaseSaved
	}
	s.recentChoice = s.choiceForID(current.ID)
	return s
}

func (s *app) view(c *ui.Context) {
	if s.alertOpen {
		s.alertScreen(c)
		return
	}
	c.SetTheme(warmTheme(c.Theme().Dark))
	t := c.Theme()
	if s.pinWin == nil || !s.pinWin.IsVisible() {
		s.showPendingToast(c)
	}

	if c.Shortcut(0, ui.KeyEscape) || c.Shortcut(ui.Cmd, ui.KeyW) {
		s.dismiss()
	}
	if s.section == 0 && c.Shortcut(ui.Cmd, ui.KeyS) {
		s.saveNow()
	}
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)

	ui.Column(c).Fill().Background(t.Background).Padding(16).Gap(12).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			brand := ui.Column(c.Key("update-check")).Grow(1).Gap(2).
				Focusable().
				Role(ui.RoleButton).
				Label("检查 Mote 更新").
				Description("点击从 GitHub 检查并安装新版本").
				Tooltip("检查更新").
				Cursor(ui.CursorPointer)
			brand.Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
					ui.Text(c, "Mote").Font("New York").FontSize(20).Bold()
					if s.updatePhase != updateIdle {
						ui.Spinner(c).Size(12, 12).TextColor(t.TextMuted).Label("正在更新 Mote")
					}
				})
				detail := "记录想法，也留一段专注时间"
				if updateStatus := s.updateStatusText(); updateStatus != "" {
					detail = updateStatus
				}
				ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted).
					ContentTransition(ui.ContentTransition{Numeric: true, Animation: &ui.Animation{Duration: 140 * time.Millisecond, Ease: ui.EaseOut}})
			})
			pose := ui.Pose{}
			if brand.Pressed() {
				pose.Scale = 0.97
			}
			brand.AnimatePose(pose, ui.Animation{Duration: 120 * time.Millisecond, Ease: ui.EaseOut})
			if brand.Clicked() || brand.Shortcut(0, ui.KeyEnter) || brand.Shortcut(0, ui.KeySpace) {
				s.checkForUpdate()
			}
			ui.Tabs(c.Key("section"), &s.section, "记录", "专注").
				Label("Mote 功能")
		})

		if s.section == 0 {
			s.noteView(c)
		} else {
			s.timerView(c)
		}
	})
}

func (s *app) noteView(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		choices := s.recentLabels()
		if ui.Select(c.Key("recent"), &s.recentChoice, choices).
			Grow(1).
			Label("最近记录").
			Disabled(s.phase == phaseDirty || s.phase == phaseSaving).
			Changed() {
			s.loadChoice(s.recentChoice)
		}
		ui.Segmented(c.Key("mode"), &s.viewMode, "编辑", "预览").Label("显示模式")
	})

	if s.viewMode == 0 {
		if ui.TextArea(c.Key("draft"), &s.draft).
			Bind(&s.editor).
			Grow(1).
			FillWidth().
			Font("Menlo").
			FontSize(13).
			LineHeight(1.55).
			Label("Markdown 内容").
			Placeholder("# 今天想到的事\n\n从这里开始写……").
			Changed() {
			s.draftChanged()
		}
	} else {
		ui.Scroll(c.Key("preview")).
			Grow(1).
			FillWidth().
			Background(t.Surface).
			Radius(10).
			Padding(16).
			Children(func() {
				if strings.TrimSpace(s.draft) == "" {
					ui.Text(c, "还没有内容").TextColor(t.TextMuted).Italic()
					return
				}
				markdown.View(c, s.draft, &s.md)
			})
	}

	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		databasePath := s.store.Path()
		status := s.statusView(c).
			Focusable().
			Role(ui.RoleButton).
			Description("点击复制数据库文件地址").
			Tooltip(databasePath).
			Cursor(ui.CursorPointer)
		if status.Clicked() || status.Shortcut(0, ui.KeyEnter) || status.Shortcut(0, ui.KeySpace) {
			c.WriteClipboard(databasePath)
			c.Toast("数据库地址已复制")
		}
		ui.Icon(c, infoIcon).
			Size(14, 14).
			TextColor(t.TextMuted.Alpha(0.65)).
			Focusable().
			Role(ui.RoleButton).
			Label("快捷键信息").
			Tooltip("⌘S 保存 · ⎋ 收起 · ⇧⌘M 呼出")
		ui.Spacer(c)
		canStartNew := (strings.TrimSpace(s.draft) == "" && s.currentID == 0) || s.phase == phaseSaved
		if ui.Button(c, "新建").Disabled(!canStartNew).Tooltip("当前内容保存后可新建").Clicked() {
			s.newDraft()
		}
		if ui.Toggle(c, &s.pinned, "钉住").
			Tooltip("保持窗口在最前，失焦也不收起").
			Changed() {
			s.applyPinned()
		}
		if ui.Button(c, "删除").
			TextColor(t.Danger).
			Disabled(s.currentID == 0 || s.phase == phaseSaving).
			Tooltip("删除这条记录").
			Clicked() {
			s.confirmDelete = true
		}
	})

	switch ui.AlertDialog(c, &s.confirmDelete, deleteDialogTitle, deleteDialogMessage,
		deleteDialogButtons[deleteDialogDelete], deleteDialogButtons[deleteDialogCancel]) {
	case deleteDialogDelete:
		s.deleteCurrent()
	}
}

func (s *app) statusView(c *ui.Context) ui.Element {
	t := c.Theme()
	label, col := "新记录尚未写入", t.TextMuted
	switch s.phase {
	case phaseDirty:
		label, col = "等待自动保存", t.Warning
	case phaseSaving:
		label, col = "正在保存", t.Warning
	case phaseSaved:
		label, col = "已保存", t.Success
		if !s.lastSavedAt.IsZero() {
			label += " · " + s.lastSavedAt.Format("15:04")
		}
	case phaseError:
		label, col = "保存失败", t.Danger
		if s.statusDetail != "" {
			label += " · " + s.statusDetail
		}
	}
	row := ui.Row(c).AlignItems(ui.Center).Gap(6)
	row.Children(func() {
		switch s.phase {
		case phaseSaving:
			ui.Spinner(c).Size(11, 11).TextColor(col).Label("正在保存")
		default:
			ui.Box(c).Size(7, 7).Radius(4).Background(col)
		}
		ui.Text(c, label).
			SingleLine().
			FontSize(11).
			TextColor(col).
			ContentTransition(ui.ContentTransition{Animation: &ui.Animation{Duration: 140 * time.Millisecond, Ease: ui.EaseOut}})
	})
	return row
}

func warmTheme(dark bool) *ui.Theme {
	if dark {
		t := ui.DarkTheme()
		t.Background = ui.Oklch(0.20, 0.012, 82)
		t.Surface = ui.Oklch(0.25, 0.014, 82)
		t.SurfaceHover = ui.Oklch(0.29, 0.018, 82)
		t.SurfacePressed = ui.Oklch(0.32, 0.022, 82)
		t.Border = ui.Oklch(0.36, 0.018, 82)
		t.Text = ui.Oklch(0.92, 0.016, 86)
		t.TextMuted = ui.Oklch(0.68, 0.018, 82)
		t.Accent = ui.Oklch(0.78, 0.14, 84)
		t.AccentHover = ui.Oklch(0.82, 0.13, 84)
		t.AccentPressed = ui.Oklch(0.72, 0.13, 84)
		t.AccentText = ui.Oklch(0.22, 0.03, 75)
		t.Selection = t.Accent.Alpha(0.35)
		t.Radius = 8
		t.Spacing = 4
		return t
	}
	t := ui.LightTheme()
	t.Background = ui.Oklch(0.975, 0.014, 88)
	t.Surface = ui.Oklch(0.995, 0.008, 88)
	t.SurfaceHover = ui.Oklch(0.965, 0.025, 88)
	t.SurfacePressed = ui.Oklch(0.935, 0.035, 88)
	t.Border = ui.Oklch(0.875, 0.025, 84)
	t.Text = ui.Oklch(0.27, 0.022, 72)
	t.TextMuted = ui.Oklch(0.53, 0.025, 76)
	t.Accent = ui.Oklch(0.82, 0.145, 84)
	t.AccentHover = ui.Oklch(0.78, 0.155, 84)
	t.AccentPressed = ui.Oklch(0.74, 0.145, 84)
	t.AccentText = ui.Oklch(0.25, 0.035, 72)
	t.Warning = ui.Oklch(0.62, 0.13, 67)
	t.Success = ui.Oklch(0.56, 0.10, 148)
	t.Selection = t.Accent.Alpha(0.32)
	t.Radius = 8
	t.Spacing = 4
	return t
}

func (s *app) draftChanged() {
	defer s.refresh()
	s.revision++
	s.statusDetail = ""
	if s.draft == s.savedText {
		if s.currentID == 0 && strings.TrimSpace(s.draft) == "" {
			s.phase = phaseIdle
		} else {
			s.phase = phaseSaved
		}
		return
	}
	s.phase = phaseDirty
	s.scheduleSave()
}

func (s *app) scheduleSave() {
	if s.autoSaveTimer != nil {
		s.autoSaveTimer.Stop()
	}
	s.autoSaveTimer = time.AfterFunc(autoSaveWait, func() {
		s.update(s.startSave)
	})
}

func (s *app) saveNow() {
	if s.autoSaveTimer != nil {
		s.autoSaveTimer.Stop()
	}
	s.startSave()
}

func (s *app) startSave() {
	if s.currentID == 0 && strings.TrimSpace(s.draft) == "" {
		s.phase = phaseIdle
		return
	}
	if s.phase == phaseSaving {
		s.savePending = true
		return
	}
	if s.draft == s.savedText && s.currentID != 0 {
		s.phase = phaseSaved
		return
	}

	id, draft, rev := s.currentID, s.draft, s.revision
	s.phase = phaseSaving
	s.statusDetail = ""
	go func() {
		saved, err := s.store.Save(context.Background(), id, draft)
		var recent []noteSummary
		if err == nil {
			recent, err = s.store.Recent(context.Background(), 12)
		}
		s.update(func() { s.finishSave(rev, draft, saved, recent, err) })
	}()
}

func (s *app) finishSave(rev uint64, draft string, saved note, recent []noteSummary, err error) {
	if err != nil {
		s.phase = phaseError
		s.statusDetail = "数据库写入失败"
		log.Println("save note:", err)
		if s.quitAfterSave {
			relaunch := s.relaunchAfterSave
			s.quitAfterSave = false
			s.relaunchAfterSave = false
			if relaunch {
				s.updatePhase = updateIdle
				s.toast = "新版本已安装，但记录保存失败；请稍后重新打开 Mote"
			} else {
				s.toast = "记录保存失败，已取消退出"
			}
		}
		return
	}

	s.currentID = saved.ID
	s.savedText = draft
	s.lastSavedAt = saved.UpdatedAt
	s.recent = recent
	s.recentChoice = s.choiceForID(saved.ID)
	if rev == s.revision && s.draft == draft {
		s.phase = phaseSaved
	} else {
		s.phase = phaseDirty
	}

	if s.quitAfterSave {
		if s.phase == phaseDirty {
			s.startSave()
			return
		}
		relaunch := s.relaunchAfterSave
		s.quitAfterSave = false
		s.relaunchAfterSave = false
		s.quitting = true
		if relaunch {
			mygo.App.Relaunch()
		} else {
			mygo.App.Quit()
		}
		return
	}
	if s.savePending || s.phase == phaseDirty {
		s.savePending = false
		s.scheduleSave()
	}
}

// deleteCurrent removes the record from the database and returns the editor
// to a fresh record, so a deleted note cannot be written back by a pending
// autosave.
func (s *app) deleteCurrent() {
	defer s.refresh()
	id := s.currentID
	if id == 0 {
		return
	}
	if s.autoSaveTimer != nil {
		s.autoSaveTimer.Stop()
	}
	if err := s.store.Delete(context.Background(), id); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.phase = phaseError
		s.statusDetail = "删除失败"
		log.Println("delete note:", err)
		return
	}

	s.currentID = 0
	s.draft = ""
	s.savedText = ""
	s.lastSavedAt = time.Time{}
	s.phase = phaseIdle
	s.statusDetail = ""
	s.revision++
	s.viewMode = 0
	if recent, err := s.store.Recent(context.Background(), 12); err == nil {
		s.recent = recent
	} else {
		log.Println("reload recent notes:", err)
	}
	s.recentChoice = "新记录"
	s.editor.Focus()
}

func (s *app) newDraft() {
	defer s.refresh()
	if s.phase == phaseDirty || s.phase == phaseSaving {
		return
	}
	s.currentID = 0
	s.draft = ""
	s.savedText = ""
	s.lastSavedAt = time.Time{}
	s.phase = phaseIdle
	s.statusDetail = ""
	s.revision++
	s.viewMode = 0
	s.recentChoice = "新记录"
	s.editor.Focus()
}

func (s *app) recentLabels() []string {
	labels := []string{"新记录"}
	for _, item := range s.recent {
		labels = append(labels, recentLabel(item))
	}
	return labels
}

func recentLabel(item noteSummary) string {
	return item.Title + "  ·  " + item.UpdatedAt.Format("01-02 15:04") + "  #" + strconv.FormatInt(item.ID, 10)
}

func (s *app) choiceForID(id int64) string {
	if id == 0 {
		return "新记录"
	}
	for _, item := range s.recent {
		if item.ID == id {
			return recentLabel(item)
		}
	}
	return "新记录"
}

func (s *app) loadChoice(choice string) {
	defer s.refresh()
	if choice == "新记录" {
		s.newDraft()
		return
	}
	for _, item := range s.recent {
		if recentLabel(item) != choice {
			continue
		}
		loaded, err := s.store.Load(context.Background(), item.ID)
		if err != nil {
			s.phase = phaseError
			s.statusDetail = "无法读取记录"
			log.Println("load note:", err)
			return
		}
		s.currentID = loaded.ID
		s.draft = loaded.Markdown
		s.savedText = loaded.Markdown
		s.lastSavedAt = loaded.UpdatedAt
		s.phase = phaseSaved
		s.statusDetail = ""
		s.revision++
		s.editor.Focus()
		return
	}
}

// applyPinned opens or closes the pinned window: a window of its own, which
// the user can move out of the way, that stays above the other apps.
func (s *app) applyPinned() {
	if s.pinWin == nil {
		return
	}
	if s.pinned {
		if !s.pinPlaced {
			s.placePinned()
			s.pinPlaced = true
		} else {
			s.pinWin.SetBounds(visibleWindowBounds(s.pinWin.Bounds()))
		}
		s.pinWin.Show()
		s.pinWin.Focus()
		s.pinEditor.Focus()
		if s.win.IsVisible() {
			s.win.Hide()
		}
		return
	}
	s.pinWin.Hide()
}

// placePinned puts a new pinned window near the top right of the main screen,
// where it covers little until the user drags it.
func (s *app) placePinned() {
	area := mygo.Screen.PrimaryDisplay().WorkArea
	bounds := s.pinWin.Bounds()
	x := area.X + area.Width - bounds.Width - 24
	y := area.Y + 24
	s.pinWin.SetPosition(
		max(area.X, min(x, area.X+max(0, area.Width-bounds.Width))),
		max(area.Y, min(y, area.Y+max(0, area.Height-bounds.Height))),
	)
}

// unpin closes the pinned window and leaves the pin off in the panel.
func (s *app) unpin() {
	s.pinned = false
	s.applyPinned()
	s.refresh()
}

// refresh redraws both windows after a change of the state they share.
func (s *app) refresh() {
	if s.win != nil {
		s.win.Invalidate()
	}
	if s.pinWin != nil && s.pinWin.IsVisible() {
		s.pinWin.Invalidate()
	}
}

// update runs fn on the main thread, where the views build, and redraws both
// windows: they show the same draft.
func (s *app) update(fn func()) {
	if s.win == nil {
		fn()
		return
	}
	s.win.Update(func() {
		fn()
		s.refresh()
	})
}

func (s *app) dismiss() {
	if s.phase == phaseDirty {
		s.saveNow()
	}
	s.win.Hide()
}

func (s *app) show() {
	if s.alertOpen && s.alertWindow != nil {
		s.alertWindow.Show()
		s.alertWindow.Focus()
		return
	}
	anchor := s.tray.Bounds()
	if anchor.Width == 0 || anchor.Height == 0 {
		s.win.Center()
	} else {
		area := mygo.Screen.DisplayNearestPoint(mygo.Point{
			X: anchor.X + anchor.Width/2,
			Y: anchor.Y + anchor.Height/2,
		}).WorkArea
		bounds := s.win.Bounds()
		x := anchor.X + (anchor.Width-bounds.Width)/2
		y := anchor.Y + anchor.Height + 7
		if y+bounds.Height > area.Y+area.Height {
			y = anchor.Y - bounds.Height - 7
		}
		x = max(area.X, min(x, area.X+max(0, area.Width-bounds.Width)))
		y = max(area.Y, min(y, area.Y+max(0, area.Height-bounds.Height)))
		s.win.SetPosition(x, y)
	}
	s.win.Show()
	if s.section == 0 && s.viewMode == 0 {
		s.editor.Focus()
	}
}

func (s *app) toggle() {
	if s.alertOpen && s.alertWindow != nil {
		s.alertWindow.Show()
		s.alertWindow.Focus()
		return
	}
	if s.win.IsVisible() {
		s.dismiss()
		return
	}
	s.show()
}

func (s *app) ready() {
	var err error
	s.tray, err = mygo.NewTray(mygo.TrayOptions{
		Icon:           trayIcon(),
		IconIsTemplate: true,
		ToolTip:        "Mote · 点击记录或专注",
	})
	if err != nil {
		log.Fatal("tray: ", err)
	}

	s.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             appName,
		Width:             460,
		Height:            540,
		MinWidth:          400,
		MinHeight:         440,
		Hidden:            true,
		Frameless:         true,
		Panel:             true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		BackgroundColor:   "light-dark(#fbf7ea, #24211c)",
		Transparent:       true,
		Vibrancy:          mygo.VibrancyHUD,
		Content:           ui.View(s.view),
	})

	s.win.OnBlur(func() {
		if s.alertOpen && s.alertWindow == s.win {
			return
		}
		s.dismiss()
	})

	s.pinWin = mygo.NewWindow(mygo.WindowOptions{
		Title:             appName + " · 钉住",
		Width:             420,
		Height:            480,
		MinWidth:          340,
		MinHeight:         280,
		Hidden:            true,
		AlwaysOnTop:       true,
		DisableMinimize:   true,
		DisableFullScreen: true,
		SkipTaskbar:       true,
		BackgroundColor:   "light-dark(#fbf7ea, #24211c)",
		Transparent:       true,
		Vibrancy:          mygo.VibrancyHUD,
		Content:           ui.View(s.pinnedView),
	})
	s.pinWin.OnClose(func(e *mygo.CloseEvent) {
		if s.quitting {
			return
		}
		e.PreventDefault()
		s.unpin()
	})
	s.win.OnClose(func(e *mygo.CloseEvent) {
		if !s.quitting {
			e.PreventDefault()
			s.dismiss()
		}
	})

	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开 Mote", Accelerator: globalHotkey, Click: func(*mygo.MenuItem, *mygo.Window) { s.show() }},
		{Label: "打开专注", Click: func(*mygo.MenuItem, *mygo.Window) { s.section = 1; s.show() }},
		{Label: "立即保存", Accelerator: "CmdOrCtrl+S", Click: func(*mygo.MenuItem, *mygo.Window) { s.saveNow() }},
		{Label: "新建记录", Accelerator: "CmdOrCtrl+N", Click: func(*mygo.MenuItem, *mygo.Window) { s.newDraft(); s.show() }},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	})

	s.tray.OnClick(s.toggle)
	s.tray.OnRightClick(func() {
		s.dismiss()
		s.tray.SetMenu(menu)
		defer s.tray.SetMenu(nil)
		s.tray.PopUpMenu()
	})
	if err := mygo.GlobalShortcut.Register(globalHotkey, s.toggle); err != nil {
		log.Println("global shortcut:", err)
	}
	if s.timer.Status == timerActive {
		if time.Now().Before(s.timer.EndsAt) {
			s.scheduleTimer()
		} else {
			s.completeTimer()
		}
	}

	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		s.stopTimerSchedule()
		mygo.GlobalShortcut.UnregisterAll()
		s.tray.Destroy()
		if err := s.store.Close(); err != nil {
			log.Println("close database:", err)
		}
	})
}

func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	ink := color.NRGBA{R: 185, G: 135, B: 18, A: 255}
	if runtime.GOOS == "darwin" {
		ink = color.NRGBA{A: 255}
	}
	for _, rect := range []image.Rectangle{
		image.Rect(6, 3, 26, 6), image.Rect(6, 26, 26, 29),
		image.Rect(6, 6, 9, 26), image.Rect(23, 6, 26, 26),
		image.Rect(12, 10, 21, 12), image.Rect(12, 16, 21, 18), image.Rect(12, 22, 18, 24),
	} {
		draw.Draw(img, rect, image.NewUniform(ink), image.Point{}, draw.Src)
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

func main() {
	mygo.App.SetName(appName)
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)

	st, err := openStore()
	if err != nil {
		log.Fatal("open database: ", err)
	}
	current, err := st.LoadCurrent(context.Background())
	if err != nil {
		_ = st.Close()
		log.Fatal("load current note: ", err)
	}
	recent, err := st.Recent(context.Background(), 12)
	if err != nil {
		_ = st.Close()
		log.Fatal("load recent notes: ", err)
	}
	timer, err := st.LoadTimerState(context.Background())
	if err != nil {
		_ = st.Close()
		log.Fatal("load timer state: ", err)
	}
	todayPomodoros, err := st.TodayPomodoros(context.Background(), time.Now())
	if err != nil {
		_ = st.Close()
		log.Fatal("load timer count: ", err)
	}
	s := newApp(st, current, recent, timer, todayPomodoros)
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !s.quitting && s.updatePhase == updateInstalling {
			e.PreventDefault()
			s.toast = "正在安装更新，请稍候"
			s.refresh()
			return
		}
		if !s.quitting && s.hasUnsavedDraft() {
			e.PreventDefault()
			s.quitAfterSave = true
			if s.phase != phaseSaving {
				s.startSave()
			}
			return
		}
		s.quitting = true
	})
	mygo.App.WhenReady(s.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
