package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/display"
)

func (a *app) overviewPage(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
		title, col, _ := a.remoteStatus(t)
		a.featureTile(c, pageRemote, title, col)
		title, col = a.hidpiSummary(t)
		a.featureTile(c, pageHiDPI, title, col)
	})

	ds := a.displays
	card(c, "排列", func() {
		if len(ds) == 0 {
			ui.Text(c, "—").TextColor(t.TextMuted)
			return
		}
		ui.Box(c).Height(190).Radius(8).Background(t.Surface.Alpha(0.5)).Clip().
			Draw(func(p *ui.Painter, r ui.Rect) { drawArrangement(p, r, ds, t) })
	})
	card(c, "显示器", func() {
		for i, d := range ds {
			if i > 0 {
				ui.Divider(c)
			}
			a.displayRow(c.Key(d.ID), d)
		}
	})
}

// featureTile is one feature's state on the overview; a click opens its page.
func (a *app) featureTile(c *ui.Context, id, status string, col ui.Color) {
	t := c.Theme()
	p := pageOf(id)
	ui.Column(c).Grow(1).Basis(0).Padding(16).Gap(10).Radius(12).
		Background(t.Background).Border(1, t.Border).
		Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04)).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(30, 30).Radius(8).Background(t.Accent.Alpha(0.12)).Center().
				Children(func() { ui.Icon(c, p.icon).Size(18, 18).TextColor(t.Accent) })
			ui.Text(c, p.label).FontSize(15).Bold().Grow(1)
			if ui.Button(c, "打开").Clicked() {
				a.page = id
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			dot(c, col, 8)
			ui.Text(c, status).FontSize(13)
		})
		ui.Text(c, p.desc).FontSize(11).TextColor(t.TextMuted).Wrap()
	})
}

func displayName(d display.Display) string {
	switch {
	case d.Virtual:
		return "远程扩展屏（虚拟）"
	case d.Builtin:
		return "内置显示器"
	default:
		return fmt.Sprintf("外接显示器 #%d", d.ID)
	}
}

func (a *app) displayRow(c *ui.Context, d display.Display) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, displayName(d)).FontSize(13).Bold()
			ui.Text(c, fmt.Sprintf("看起来像 %d×%d · %d×%d px · %.0f Hz", d.Width, d.Height, d.PixelWidth, d.PixelHeight, d.RefreshHz)).
				FontSize(12).TextColor(t.TextMuted)
		})
		if d.Main {
			ui.Badge(c, "主显示器").Background(t.Accent.Alpha(0.12)).TextColor(t.Accent)
		}
		if d.HiDPI() {
			ui.Badge(c, "HiDPI")
		}
		if d.MirrorOf != 0 {
			ui.Badge(c, fmt.Sprintf("镜像 #%d", d.MirrorOf))
		}
		if d.Virtual {
			ui.Badge(c, "虚拟").Background(t.Success.Alpha(0.15)).TextColor(t.Success)
		}
	})
}

// drawArrangement draws the displays where they sit in the desktop, as in
// System Settings. Mirrors share their source's place and are not drawn.
func drawArrangement(p *ui.Painter, r ui.Rect, ds []display.Display, t *ui.Theme) {
	var shown []display.Display
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, d := range ds {
		if d.MirrorOf != 0 || d.Width == 0 {
			continue
		}
		shown = append(shown, d)
		minX, minY = math.Min(minX, float64(d.X)), math.Min(minY, float64(d.Y))
		maxX, maxY = math.Max(maxX, float64(d.X+d.Width)), math.Max(maxY, float64(d.Y+d.Height))
	}
	if len(shown) == 0 {
		return
	}
	const pad = 20
	scale := math.Min((float64(r.W)-2*pad)/(maxX-minX), (float64(r.H)-2*pad)/(maxY-minY))
	offX := float64(r.X) + (float64(r.W)-(maxX-minX)*scale)/2
	offY := float64(r.Y) + (float64(r.H)-(maxY-minY)*scale)/2
	for _, d := range shown {
		box := ui.Rect{
			X: float32(offX + (float64(d.X)-minX)*scale + 2), Y: float32(offY + (float64(d.Y)-minY)*scale + 2),
			W: float32(float64(d.Width)*scale - 4), H: float32(float64(d.Height)*scale - 4),
		}
		fill, edge := t.Background, t.Border
		switch {
		case d.Virtual:
			fill, edge = t.Success.Alpha(0.12), t.Success
		case d.Main:
			edge = t.Accent
		}
		p.Fill(box, fill, 6)
		p.Stroke(box, edge, 6, 1.5)
		if d.Main {
			// The menu bar.
			p.Fill(ui.Rect{X: box.X + 1.5, Y: box.Y + 1.5, W: box.W - 3, H: 5}, t.Accent.Alpha(0.35), 0)
		}
		if box.W > 70 && box.H > 36 {
			p.Text(box.X+8, box.Y+11, displayName(d), 11, t.Text)
			p.Text(box.X+8, box.Y+28, fmt.Sprintf("%d×%d", d.Width, d.Height), 10, t.TextMuted)
		}
	}
}
