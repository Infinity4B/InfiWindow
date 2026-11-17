package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"

	"github.com/egoist/mygo"

	"github.com/Infinity4B/infiwindow/internal/config"
)

func (a *app) setupTray() {
	a.trayStatus = &mygo.MenuItem{Label: "正在启动…", Disabled: true}
	a.trayToggle = &mygo.MenuItem{Label: "立即创建虚拟屏", Click: func(*mygo.MenuItem, *mygo.Window) {
		if a.snap.DisplayOn {
			a.ctl.RemoveNow()
		} else {
			a.ctl.CreateNow()
		}
	}}
	a.trayAuto = &mygo.MenuItem{Label: "UU 远程连接时自动创建", Type: mygo.MenuItemCheckbox,
		Checked: a.edit.Mode == config.ModeAuto,
		Click: func(it *mygo.MenuItem, _ *mygo.Window) {
			if it.IsChecked() {
				a.edit.Mode = config.ModeAuto
			} else {
				a.edit.Mode = config.ModeManual
			}
			a.ctl.SetConfig(a.edit)
			a.win.Invalidate()
		}}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		a.trayStatus,
		mygo.Separator(),
		{Label: "打开 InfiWindow", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		a.trayToggle,
		a.trayAuto,
		mygo.Separator(),
		{Label: "退出 InfiWindow", Accelerator: "CmdOrCtrl+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		ID:             "infiwindow",
		Icon:           trayIcon(false),
		IconIsTemplate: true,
		ToolTip:        "InfiWindow",
		Menu:           menu,
	})
	if err != nil {
		log.Printf("tray: %v", err)
	}
}

// updateTray mirrors the snapshot into the menu bar item. Main thread.
func (a *app) updateTray() {
	if a.tray == nil {
		return
	}
	s := a.snap
	status := "等待 UU 远程连接"
	switch {
	case s.Busy:
		status = "正在调整显示器…"
	case s.DisplayOn:
		status = fmt.Sprintf("虚拟屏运行中 · %d×%d", s.Spec.Selected.Width, s.Spec.Selected.Height)
	case s.Connected:
		status = "检测到远程会话"
	case s.Config.Mode == config.ModeManual:
		status = "手动模式"
	}
	a.trayStatus.SetLabel(status)
	if s.DisplayOn {
		a.trayToggle.SetLabel("移除虚拟屏")
	} else {
		a.trayToggle.SetLabel("立即创建虚拟屏")
	}
	a.trayAuto.SetChecked(s.Config.Mode == config.ModeAuto)
	if a.trayOn != s.DisplayOn || !a.trayInit {
		a.trayOn, a.trayInit = s.DisplayOn, true
		_ = a.tray.SetIcon(trayIcon(s.DisplayOn), true)
		a.tray.SetToolTip("InfiWindow · " + status)
	}
}

// trayIcon draws an iPad in landscape, 18×18 pt at 2x, black on transparent
// for macOS to tint. When the display is on the screen is filled.
func trayIcon(on bool) []byte {
	const size = 36
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	// Rounded rectangle body.
	x0, y0, x1, y1 := 3.0, 7.0, 33.0, 29.0
	r := 4.0
	stroke := 2.6
	for y := range size {
		for x := range size {
			px, py := float64(x)+0.5, float64(y)+0.5
			d := roundedRectDist(px, py, x0, y0, x1, y1, r)
			var alpha float64
			if on {
				// Outline plus a filled screen with a small gap.
				alpha = coverage(-d) - coverage(-(d + stroke)) + coverage(-(d + stroke + 1.6))
			} else {
				alpha = coverage(-d) - coverage(-(d + stroke))
			}
			alpha = math.Max(0, math.Min(1, alpha))
			img.SetNRGBA(x, y, color.NRGBA{A: uint8(alpha * 255)})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// roundedRectDist is the signed distance from (px,py) to a rounded rectangle:
// negative inside.
func roundedRectDist(px, py, x0, y0, x1, y1, r float64) float64 {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hw, hh := (x1-x0)/2-r, (y1-y0)/2-r
	dx := math.Abs(px-cx) - hw
	dy := math.Abs(py-cy) - hh
	outside := math.Hypot(math.Max(dx, 0), math.Max(dy, 0))
	inside := math.Min(math.Max(dx, dy), 0)
	return outside + inside - r
}

// coverage antialiases a distance (positive = inside) into 0..1.
func coverage(d float64) float64 {
	return math.Max(0, math.Min(1, d+0.5))
}
