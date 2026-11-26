package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/remote"
	"github.com/Infinity4B/infiwindow/internal/remote/device"
)

var accent = ui.Hex("#3D7EFF")

// Pages of the window, one per feature plus the shared ones.
const (
	pageOverview = "overview"
	pageRemote   = "remote"
	pageHiDPI    = "hidpi"
	pageLog      = "log"
	pageSettings = "settings"
)

func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	iconOverview = icon(`<rect x="1.5" y="2.5" width="13" height="8.5" rx="1.5"/><path d="M5.5 13.5h5M8 11v2.5"/>`)
	iconRemote   = icon(`<rect x="1.5" y="3" width="13" height="10" rx="2"/><path d="M12.5 7.2v1.6"/>`)
	iconHiDPI    = icon(`<rect x="2" y="2" width="12" height="12" rx="2"/><path d="M2 8h12M8 2v12"/>`)
	iconLog      = icon(`<path d="M5.5 4h8M5.5 8h8M5.5 12h8M2.5 4h.01M2.5 8h.01M2.5 12h.01"/>`)
	iconSettings = icon(`<path d="M2 4.5h3M8 4.5h6M2 11.5h7M12 11.5h2"/><circle cx="6.5" cy="4.5" r="1.5"/><circle cx="10.5" cy="11.5" r="1.5"/>`)
)

type pageInfo struct {
	id, label, desc string
	icon            *ui.SVG
}

var pages = []pageInfo{
	{pageOverview, "显示器", "这台 Mac 当前连接的所有显示器，以及各项功能的状态。", iconOverview},
	{pageRemote, "远程扩展屏", "用 UU 远程从 iPad 或电脑连入时，自动加一块匹配对方分辨率的虚拟屏，画面铺满不留黑边。", iconRemote},
	{pageHiDPI, "外接屏 HiDPI", "给 2K 等非 Retina 外接显示器加上 HiDPI 缩放档位，文字和图标更平滑。", iconHiDPI},
	{pageLog, "活动记录", "各项功能做过的操作。", iconLog},
	{pageSettings, "设置", "", iconSettings},
}

func pageOf(id string) pageInfo {
	for _, p := range pages {
		if p.id == id {
			return p
		}
	}
	return pages[0]
}

func (a *app) view(c *ui.Context) {
	th := *c.Theme().WithAccent(accent)
	th.Radius = 8
	c.SetTheme(&th)
	t := c.Theme()

	if c.Shortcut(ui.Cmd, ui.KeyW) {
		a.win.Hide()
	}
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(212).Shrink(0).Background(t.Surface).Children(func() { a.sidebar(c) })
		ui.Box(c).Width(1).Background(t.Border)
		ui.Column(c).Grow(1).MinWidth(0).Background(t.Background.Mix(t.Text, 0.04)).Children(func() {
			p := pageOf(a.page)
			a.pageHeader(c, p)
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Padding(4, 24, 24, 24).Gap(16).Children(func() {
					switch p.id {
					case pageRemote:
						a.remotePage(c)
					case pageHiDPI:
						a.hidpiPage(c)
					case pageLog:
						a.logPage(c)
					case pageSettings:
						a.settingsPage(c)
					default:
						a.overviewPage(c)
					}
				})
			})
		})
	})
}

// sidebar is the app's name under the window controls, then the pages.
func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	ui.Box(c).Height(max(bar.Height, 28)).Shrink(0).DragWindow()
	ui.Row(c).Padding(4, 16, 12, 16).Gap(10).AlignItems(ui.Center).DragWindow().Children(func() {
		logo(c, 30)
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, "InfiWindow").FontSize(15).Bold()
			ui.Text(c, "Mac 显示器工具箱").FontSize(11).TextColor(t.TextMuted)
		})
	})
	ui.Sidebar(c, &a.page, func() {
		ui.SidebarSection(c, "概览", nil, func() {
			a.sidebarItem(c, pageOverview)
		})
		ui.SidebarSection(c, "功能", nil, func() {
			a.sidebarItem(c, pageRemote)
			a.sidebarItem(c, pageHiDPI)
		})
		ui.SidebarSection(c, "应用", nil, func() {
			a.sidebarItem(c, pageLog)
			a.sidebarItem(c, pageSettings)
		})
	}).Grow(1).Background(ui.Transparent).Label("页面")
}

func (a *app) sidebarItem(c *ui.Context, id string) {
	t := c.Theme()
	p := pageOf(id)
	item := ui.SidebarItem(c, p.id, p.icon, p.label)
	switch id {
	case pageRemote:
		if a.snap.DisplayOn {
			item.Children(func() { ui.Box(c).Size(8, 8).Radius(4).Background(t.Success) })
		}
	case pageHiDPI:
		if n := a.hidpiActive(); n > 0 {
			item.Children(func() { ui.Box(c).Size(8, 8).Radius(4).Background(t.Success) })
		}
	}
}

// logo is the app icon: a window with a second one behind it.
func logo(c *ui.Context, size float32) {
	ui.Box(c).Size(size, size).Radius(size*0.26).Shrink(0).
		Gradient(ui.Hex("#5B9BFF"), ui.Hex("#2F5BEA"), 135).
		Draw(func(p *ui.Painter, r ui.Rect) {
			u := r.W / 34
			back := ui.Rect{X: r.X + 11*u, Y: r.Y + 7*u, W: 16 * u, H: 12 * u}
			p.Fill(back, ui.RGBA(255, 255, 255, 0.45), 2*u)
			front := ui.Rect{X: r.X + 7*u, Y: r.Y + 13*u, W: 18 * u, H: 14 * u}
			p.Fill(front, ui.Hex("#FFFFFF"), 2.5*u)
			p.Fill(ui.Rect{X: front.X, Y: front.Y + 3.5*u, W: front.W, H: 1.2 * u}, ui.Hex("#2F5BEA").Alpha(0.5), 0)
		})
}

func (a *app) pageHeader(c *ui.Context, p pageInfo) {
	t := c.Theme()
	bar := c.TitleBar()
	ui.Row(c).Shrink(0).Padding(max(bar.Height, 28)-8, bar.Right+24, 14, 24).Gap(12).AlignItems(ui.End).
		DragWindow().Children(func() {
		ui.Column(c).Grow(1).Gap(4).Children(func() {
			ui.Text(c, p.label).FontSize(22).Bold()
			if p.desc != "" {
				ui.Text(c, p.desc).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
		})
		if p.id == pageRemote {
			ui.Row(c).Gap(6).Children(func() {
				ui.Badge(c, device.KindLabel(a.edit.Active)).Background(t.Surface).TextColor(t.Text)
				mode := "自动模式"
				if a.edit.Mode == remote.ModeManual {
					mode = "手动模式"
				}
				ui.Badge(c, mode).Background(t.Accent.Alpha(0.12)).TextColor(t.Accent)
			})
		}
	})
}

func card(c *ui.Context, title string, fn func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(16).Gap(12).Radius(12).
		Background(t.Background).Border(1, t.Border).
		Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04)).
		Children(func() {
			if title != "" {
				ui.Text(c, title).FontSize(13).Bold().TextColor(t.TextMuted)
			}
			fn()
		})
}

func chip(c *ui.Context, text string) {
	t := c.Theme()
	ui.Box(c).Padding(3, 8).Radius(6).Background(t.Surface).Children(func() {
		ui.Text(c, text).FontSize(12)
	})
}

func stat(c *ui.Context, label, value string) {
	t := c.Theme()
	ui.Column(c).Gap(2).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		ui.Text(c, value).FontSize(17).Bold().FontFeatures("tnum")
	})
}

func dot(c *ui.Context, col ui.Color, size float32) {
	ui.Box(c).Size(size, size).Radius(size/2).Shrink(0).Background(col).Shadow(0, 0, 6, 0, col.Alpha(0.5))
}

func indexOf[T comparable](list []T, v T) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return 0
}

func since(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t).Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0:
		return fmt.Sprintf("%d 小时 %d 分", h, m)
	case m > 0:
		return fmt.Sprintf("%d 分 %d 秒", m, s)
	default:
		return fmt.Sprintf("%d 秒", s)
	}
}
