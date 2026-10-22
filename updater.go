package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type updatePhase int

const (
	updateIdle updatePhase = iota
	updateChecking
	updateInstalling
)

type availableUpdate struct {
	version string
	install func(context.Context, func(downloaded, total int64)) error
}

// findUpdate is a small seam around MyGo's signed updater. Keeping it here
// makes the app's state transitions testable without reaching GitHub.
var findUpdate = func(ctx context.Context) (*availableUpdate, error) {
	if !mygo.Updater.Enabled() {
		return nil, mygo.ErrUpdatesDisabled
	}
	up, err := mygo.Updater.Check(ctx)
	if err != nil || up == nil {
		return nil, err
	}
	return &availableUpdate{version: up.Version, install: up.Install}, nil
}

func (s *app) checkForUpdate() {
	if s.updatePhase != updateIdle {
		return
	}
	s.updatePhase = updateChecking
	s.updateProgress = 0
	s.refresh()

	go func() {
		checkCtx, cancelCheck := context.WithTimeout(context.Background(), 30*time.Second)
		up, err := findUpdate(checkCtx)
		cancelCheck()
		if err != nil {
			s.update(func() {
				s.updatePhase = updateIdle
				if errors.Is(err, mygo.ErrUpdatesDisabled) {
					s.toast = "当前版本无法自动更新"
				} else {
					s.toast = "检查更新失败，请稍后再试"
				}
			})
			return
		}
		if up == nil {
			s.update(func() {
				s.updatePhase = updateIdle
				s.toast = fmt.Sprintf("Mote %s 已是最新版本", mygo.App.Version())
			})
			return
		}

		s.update(func() {
			s.updatePhase = updateInstalling
			s.updateVersion = up.version
			s.updateProgress = 0
		})
		installCtx, cancelInstall := context.WithTimeout(context.Background(), 15*time.Minute)
		err = up.install(installCtx, func(downloaded, total int64) {
			progress := 0
			if total > 0 {
				progress = int(downloaded * 100 / total)
			}
			s.update(func() { s.updateProgress = min(100, max(0, progress)) })
		})
		cancelInstall()
		if err != nil {
			s.update(func() {
				s.updatePhase = updateIdle
				s.updateVersion = ""
				s.updateProgress = 0
				s.toast = "升级失败，仍可继续使用当前版本"
			})
			return
		}

		s.update(func() {
			s.updateProgress = 100
			s.relaunchWhenSaved()
		})
	}()
}

func (s *app) updateStatusText() string {
	switch s.updatePhase {
	case updateChecking:
		return "正在检查 GitHub 更新"
	case updateInstalling:
		return fmt.Sprintf("正在升级到 Mote %s · %d%%", s.updateVersion, s.updateProgress)
	default:
		return ""
	}
}

func (s *app) showPendingToast(c *ui.Context) {
	if s.toast == "" {
		return
	}
	toast := s.toast
	s.toast = ""
	c.Toast(toast)
}

func (s *app) hasUnsavedDraft() bool {
	return s.phase == phaseSaving || s.draft != s.savedText
}

// relaunchWhenSaved keeps an update from interrupting a pending note save.
// The updater has already replaced the bundle at this point; the new version
// starts only after the normal quit lifecycle has closed the database.
func (s *app) relaunchWhenSaved() {
	if s.hasUnsavedDraft() {
		s.quitAfterSave = true
		s.relaunchAfterSave = true
		if s.phase != phaseSaving {
			s.startSave()
		}
		return
	}
	s.quitting = true
	mygo.App.Relaunch()
}
