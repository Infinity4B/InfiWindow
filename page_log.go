package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/activity"
)

func (a *app) logPage(c *ui.Context) {
	t := c.Theme()
	logs := a.log.Entries()
	card(c, "", func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "最近的操作，最新的在最上面").FontSize(12).TextColor(t.TextMuted).Grow(1)
			if ui.Button(c, "清空").Disabled(len(logs) == 0).Clicked() {
				a.log.Clear()
			}
		})
		if len(logs) == 0 {
			ui.Text(c, "暂无记录").FontSize(12).TextColor(t.TextMuted)
		}
		for i := len(logs) - 1; i >= 0 && i >= len(logs)-150; i-- {
			e := logs[i]
			col := t.TextMuted
			switch e.Level {
			case activity.Good:
				col = t.Success
			case activity.Warn:
				col = t.Warning
			case activity.Bad:
				col = t.Danger
			}
			ui.Row(c.Key(i)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, e.At.Format("01-02 15:04:05")).FontSize(12).TextColor(t.TextMuted).FontFeatures("tnum")
				ui.Box(c).Size(6, 6).Radius(3).Background(col)
				ui.Box(c).Width(84).Shrink(0).Children(func() {
					ui.Text(c, e.Source).FontSize(11).TextColor(t.TextMuted).SingleLine()
				})
				ui.Text(c, e.Text).FontSize(12).Grow(1).Wrap().Selectable()
			})
		}
	})
}
