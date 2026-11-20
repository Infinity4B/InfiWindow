package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) settingsPage(c *ui.Context) {
	t := c.Theme()
	card(c, "通用", func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "登录时启动").FontSize(13)
				ui.Text(c, "在菜单栏后台运行，远程扩展屏随时待命").FontSize(11).TextColor(t.TextMuted)
			})
			if ui.Switch(c, &a.openAtLogin).Label("登录时启动").Changed() {
				if err := mygo.App.SetOpenAtLogin(a.openAtLogin); err != nil {
					a.openAtLogin = !a.openAtLogin
					c.Toast("无法设置登录项：" + err.Error())
				}
			}
		})
	})
	card(c, "命令行", func() {
		ui.Text(c, "可以从脚本或快捷指令控制正在运行的 InfiWindow：").FontSize(12).TextColor(t.TextMuted).Wrap()
		for _, line := range []string{
			"InfiWindow --create   # 创建远程扩展屏",
			"InfiWindow --remove   # 移除远程扩展屏",
			"InfiWindow --quit     # 退出",
		} {
			ui.Text(c, line).FontSize(12).Font("Menlo").Selectable()
		}
		ui.Text(c, "程序位于 /Applications/InfiWindow.app/Contents/MacOS/InfiWindow。").FontSize(11).TextColor(t.TextMuted).Wrap().Selectable()
	})
	card(c, "关于", func() {
		ui.Text(c, "InfiWindow "+mygo.App.Version()).FontSize(13)
		ui.Text(c, "github.com/Infinity4B/infiwindow").FontSize(12).TextColor(t.TextMuted).Selectable()
	})
}
