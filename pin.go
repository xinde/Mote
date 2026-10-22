package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// pinnedView is the window that 钉住 opens: the same draft, in a window the
// user can drag out of the way, which stays above the other apps.
func (s *app) pinnedView(c *ui.Context) {
	if s.alertOpen {
		s.alertScreen(c)
		return
	}
	c.SetTheme(warmTheme(c.Theme().Dark))
	t := c.Theme()
	s.showPendingToast(c)

	if c.Shortcut(0, ui.KeyEscape) || c.Shortcut(ui.Cmd, ui.KeyW) {
		s.unpin()
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) {
		s.saveNow()
	}

	ui.Column(c).Fill().Background(t.Background).Padding(14).Gap(10).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Segmented(c.Key("pin-mode"), &s.pinMode, "编辑", "预览").Label("显示模式")
			ui.Spacer(c)
			ui.Button(c, "取消钉住").Tooltip("关闭这个窗口，回到菜单栏").OnClick(s.unpin)
		})
		if updateStatus := s.updateStatusText(); updateStatus != "" {
			ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
				ui.Spinner(c).Size(11, 11).TextColor(t.TextMuted).Label("正在更新 Mote")
				ui.Text(c, updateStatus).FontSize(11).TextColor(t.TextMuted)
			})
		}

		if s.pinMode == 0 {
			if ui.TextArea(c.Key("pin-draft"), &s.draft).
				Bind(&s.pinEditor).
				Grow(1).
				FillWidth().
				Font("Menlo").
				FontSize(13).
				LineHeight(1.55).
				Label("Markdown 内容").
				Placeholder("# 随手记点什么\n\n这个窗口一直留在最前，拖动它换个位置。").
				Changed() {
				s.draftChanged()
			}
		} else {
			ui.Scroll(c.Key("pin-preview")).
				Grow(1).
				FillWidth().
				Background(t.Surface).
				Radius(10).
				Padding(14).
				Children(func() {
					if strings.TrimSpace(s.draft) == "" {
						ui.Text(c, "还没有内容").TextColor(t.TextMuted).Italic()
						return
					}
					markdown.View(c, s.draft, &s.md)
				})
		}

		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			s.statusView(c)
			ui.Spacer(c)
			ui.Text(c, "自动保存").
				FontSize(10).
				TextColor(t.TextMuted)
		})
	})
}
