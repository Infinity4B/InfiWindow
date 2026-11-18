package main

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/config"
	"github.com/Infinity4B/infiwindow/internal/controller"
	"github.com/Infinity4B/infiwindow/internal/detector"
	"github.com/Infinity4B/infiwindow/internal/device"
	"github.com/Infinity4B/infiwindow/internal/vdisplay"
)

// app is the UI state. Everything here is read and written on the main
// thread; the controller goroutine only hands over snapshots via Update.
type app struct {
	win    *mygo.Window
	tray   *mygo.Tray
	ctl    *controller.Controller
	cancel context.CancelFunc

	trayStatus, trayToggle, trayAuto *mygo.MenuItem
	trayOn, trayInit                 bool

	quitting bool
	snap     controller.Snapshot
	hasSnap  bool

	// edit is the configuration the settings panel edits; every change is
	// sent to the controller, which saves it.
	edit        config.Config
	presetName  string
	openAtLogin bool
	advanced    bool
	procText    string
	procInit    bool
}

var accent = ui.Hex("#3D7EFF")

var (
	orientLabels = []string{"横屏", "竖屏"}
	scaleKeys    = []string{device.ScaleLarger, device.ScaleDefault, device.ScaleMore, device.ScaleNative}
	scaleLabels  = []string{"放大", "默认", "缩小", "原生 1:1"}
	hzValues     = []int{60, 120}
	layoutKeys   = []vdisplay.Layout{vdisplay.LayoutMirror, vdisplay.LayoutMain, vdisplay.LayoutExtend}
	layoutLabels = []string{"镜像", "主显示器", "扩展"}
	layoutHelp   = []string{
		"其他屏幕都镜像虚拟屏：UU 远程无论采集哪块屏幕，iPad 上都是铺满的画面。推荐。",
		"虚拟屏成为主显示器（菜单栏和程序坞在上面），其他屏幕保持扩展。",
		"只在旁边加一块虚拟屏，需要在 UU 远程里手动切换到它。",
	}
)

func (a *app) apply() { a.ctl.SetConfig(a.edit) }

func (a *app) syncPresetName() {
	a.presetName = device.Find(a.edit.Device.PresetID).Name
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

	page := t.Background.Mix(t.Text, 0.04)
	ui.Column(c).Fill().Background(page).Children(func() {
		a.header(c)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Row(c).Padding(4, 20, 20, 20).Gap(16).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).Basis(0).Gap(16).Children(func() {
					a.statusCard(c)
					a.trafficCard(c)
					a.displaysCard(c)
					a.logCard(c)
				})
				ui.Column(c).Width(340).Gap(16).Children(func() {
					a.settingsCard(c)
					a.detectCard(c)
				})
			})
		})
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

func (a *app) header(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	ui.Row(c).Height(max(bar.Height, 64)).Padding(0, bar.Right+20, 0, bar.Left+20).
		Gap(12).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Box(c).Size(34, 34).Radius(9).
			Gradient(ui.Hex("#5B9BFF"), ui.Hex("#2F5BEA"), 135).
			Draw(func(p *ui.Painter, r ui.Rect) {
				body := ui.Rect{X: r.X + 7, Y: r.Y + 10, W: r.W - 14, H: r.H - 20}
				p.Stroke(body, ui.Hex("#FFFFFF"), 3, 2)
				p.Fill(ui.Rect{X: body.X + 3, Y: body.Y + 3, W: body.W - 6, H: body.H - 6}, ui.RGBA(255, 255, 255, 0.55), 1.5)
			})
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, "InfiWindow").FontSize(18).Bold()
			ui.Text(c, "UU 远程连接时，自动为 iPad 准备一块刚好铺满的虚拟屏").FontSize(12).TextColor(t.TextMuted)
		})
		ui.Spacer(c)
		mode := "自动模式"
		if a.edit.Mode == config.ModeManual {
			mode = "手动模式"
		}
		ui.Badge(c, mode).Background(t.Accent.Alpha(0.12)).TextColor(t.Accent)
	})
}

func (a *app) statusCard(c *ui.Context) {
	t := c.Theme()
	s := a.snap
	card(c, "", func() {
		title, dot := "等待 UU 远程连接", t.TextMuted
		hint := "在 iPad 上用 UU 远程连接这台 Mac，InfiWindow 会自动创建虚拟屏，断开后自动移除。"
		switch {
		case !a.hasSnap:
			title, hint = "正在启动…", ""
		case s.Busy:
			title, dot, hint = "正在调整显示器…", t.Warning, "屏幕可能会闪烁一下。"
		case s.DisplayOn:
			title, dot = "虚拟屏运行中", t.Success
			hint = fmt.Sprintf("已运行 %s · 显示器 ID %d", since(s.Since), s.DisplayID)
			if s.Connected {
				hint += " · 远程会话中"
			}
		case s.Connected && s.Suppressed:
			title, dot, hint = "远程会话中 · 虚拟屏已手动移除", t.Warning, "本次会话结束前不会再自动创建。"
		case s.Connected:
			title, dot, hint = "检测到远程会话", t.Accent, "当前为手动模式，点击“立即创建虚拟屏”或切换到自动模式。"
		case s.Config.Mode == config.ModeManual:
			title, hint = "手动模式", "点击“立即创建虚拟屏”开始，或在右侧切换为自动模式。"
		}

		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(12, 12).Radius(6).Background(dot).
				Shadow(0, 0, 6, 0, dot.Alpha(0.5))
			ui.Text(c, title).FontSize(22).Bold()
		})
		if hint != "" {
			ui.Text(c, hint).FontSize(13).TextColor(t.TextMuted).Wrap()
		}

		spec := s.Spec
		if !a.hasSnap {
			spec, _ = device.BuildSpec(a.edit.Device)
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			chip(c, device.Find(a.edit.Device.PresetID).Name)
			pw, ph := spec.Selected.PixelSize()
			chip(c, fmt.Sprintf("%d×%d pt", spec.Selected.Width, spec.Selected.Height))
			chip(c, fmt.Sprintf("%d×%d px", pw, ph))
			chip(c, fmt.Sprintf("%g Hz", spec.RefreshHz))
			chip(c, layoutLabels[indexOf(layoutKeys, a.edit.Layout)])
		})

		for _, msg := range []string{s.SpecErr, s.LastErr, s.WatchErr} {
			if msg != "" {
				ui.Text(c, msg).FontSize(12).TextColor(t.Danger).Wrap()
			}
		}

		ui.Row(c).Gap(8).Children(func() {
			if s.DisplayOn {
				if ui.Button(c, "移除虚拟屏").Disabled(s.Busy).Clicked() {
					a.ctl.RemoveNow()
				}
				if ui.Button(c, "按当前设置重建").Disabled(s.Busy).
					Tooltip("在系统设置里改过分辨率后，用它恢复到 InfiWindow 的设置").Clicked() {
					a.ctl.Recreate()
				}
			} else if ui.PrimaryButton(c, "立即创建虚拟屏").Disabled(s.Busy || !a.hasSnap).Clicked() {
				a.ctl.CreateNow()
			}
		})
	})
}

func chip(c *ui.Context, text string) {
	t := c.Theme()
	ui.Box(c).Padding(3, 8).Radius(6).Background(t.Surface).Children(func() {
		ui.Text(c, text).FontSize(12)
	})
}

func (a *app) trafficCard(c *ui.Context) {
	t := c.Theme()
	s := a.snap
	card(c, "UU 远程流量", func() {
		ui.Row(c).Gap(24).Children(func() {
			stat(c, "上行", rate(s.Sample.UploadKBps))
			stat(c, "下行", rate(s.Sample.DownKBps))
			conns := "—"
			if s.Sample.Conns >= 0 {
				conns = fmt.Sprint(s.Sample.Conns)
			}
			stat(c, "对端连接", conns)
			procs := "未运行"
			if s.Sample.Processes > 0 {
				procs = fmt.Sprintf("%d 个", s.Sample.Processes)
			}
			stat(c, "UU 进程", procs)
		})
		history := s.History
		p := a.edit.Detect
		ui.Box(c).Height(120).Radius(8).Background(t.Surface.Alpha(0.5)).Clip().
			Draw(func(pt *ui.Painter, r ui.Rect) {
				drawChart(pt, r, history, p, t)
			})
		ui.Text(c, "上行持续高于“开始阈值”即判定为远程会话；之后“断开等待”时间内的平均上行低于“保持阈值”判定为断开（纵轴为平方根刻度）。").
			FontSize(11).TextColor(t.TextMuted).Wrap()
	})
}

func stat(c *ui.Context, label, value string) {
	t := c.Theme()
	ui.Column(c).Gap(2).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		ui.Text(c, value).FontSize(17).Bold().FontFeatures("tnum")
	})
}

func rate(kbps float64) string {
	switch {
	case kbps >= 1024:
		return fmt.Sprintf("%.1f MB/s", kbps/1024)
	case kbps >= 10:
		return fmt.Sprintf("%.0f KB/s", kbps)
	default:
		return fmt.Sprintf("%.1f KB/s", kbps)
	}
}

func drawChart(p *ui.Painter, r ui.Rect, history []float64, params detector.Params, t *ui.Theme) {
	maxV := params.ConnectKBps * 1.4
	for _, v := range history {
		maxV = math.Max(maxV, v)
	}
	pad := float32(8)
	inner := ui.Rect{X: r.X + pad, Y: r.Y + pad, W: r.W - 2*pad, H: r.H - 2*pad}
	y := func(v float64) float32 {
		f := math.Sqrt(math.Max(v, 0) / maxV)
		return inner.Y + inner.H*(1-float32(f))
	}
	threshold := func(v float64, label string, col ui.Color) {
		yy := y(v)
		line := (&ui.Path{}).MoveTo(inner.X, yy).LineTo(inner.X+inner.W, yy)
		p.StrokePath(line.Cap(ui.ButtCap).Dash(0, 5, 4), 1, col)
		p.Text(inner.X+4, yy-14, label, 10, col)
	}
	threshold(params.ConnectKBps, fmt.Sprintf("开始阈值 %.0f KB/s", params.ConnectKBps), t.Warning)
	if params.HoldKBps > 0 {
		threshold(params.HoldKBps, fmt.Sprintf("保持阈值 %.0f KB/s", params.HoldKBps), t.TextMuted)
	}
	if len(history) < 2 {
		p.Text(inner.X+inner.W/2-40, inner.Y+inner.H/2-6, "正在采集流量…", 11, t.TextMuted)
		return
	}
	step := inner.W / float32(controller.HistoryLen-1)
	x0 := inner.X + inner.W - step*float32(len(history)-1)
	var line, area ui.Path
	for i, v := range history {
		x := x0 + step*float32(i)
		if i == 0 {
			line.MoveTo(x, y(v))
			area.MoveTo(x, inner.Y+inner.H).LineTo(x, y(v))
		} else {
			line.LineTo(x, y(v))
			area.LineTo(x, y(v))
		}
	}
	area.LineTo(x0+step*float32(len(history)-1), inner.Y+inner.H).Close()
	p.FillPathGradient(&area, ui.LinearGradient{From: t.Accent.Alpha(0.35), To: t.Accent.Alpha(0.02), Angle: 180})
	p.StrokePath(&line, 2, t.Accent)
}

func (a *app) displaysCard(c *ui.Context) {
	t := c.Theme()
	card(c, "当前显示器", func() {
		if len(a.snap.Displays) == 0 {
			ui.Text(c, "—").TextColor(t.TextMuted)
			return
		}
		for _, d := range a.snap.Displays {
			ui.Row(c.Key(d.ID)).Gap(10).AlignItems(ui.Center).Children(func() {
				name := fmt.Sprintf("外接显示器 #%d", d.ID)
				switch {
				case d.Virtual:
					name = "InfiWindow 虚拟屏"
				case d.Builtin:
					name = "内置显示器"
				}
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, name).FontSize(13).Bold()
					ui.Text(c, fmt.Sprintf("%d×%d pt · %d×%d px · %.0f Hz", d.Width, d.Height, d.PixelWidth, d.PixelHeight, d.RefreshHz)).
						FontSize(12).TextColor(t.TextMuted)
				})
				if d.Main {
					ui.Badge(c, "主显示器").Background(t.Accent.Alpha(0.12)).TextColor(t.Accent)
				}
				if d.MirrorOf != 0 {
					ui.Badge(c, fmt.Sprintf("镜像 #%d", d.MirrorOf))
				}
				if d.Virtual {
					ui.Badge(c, "虚拟").Background(t.Success.Alpha(0.15)).TextColor(t.Success)
				}
			})
		}
	})
}

func (a *app) logCard(c *ui.Context) {
	t := c.Theme()
	card(c, "", func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "活动记录").FontSize(13).Bold().TextColor(t.TextMuted).Grow(1)
			if ui.Button(c, "清空").Disabled(len(a.snap.Logs) == 0).Clicked() {
				a.ctl.ClearLog()
			}
		})
		logs := a.snap.Logs
		if len(logs) == 0 {
			ui.Text(c, "暂无记录").FontSize(12).TextColor(t.TextMuted)
		}
		for i := len(logs) - 1; i >= 0 && i >= len(logs)-60; i-- {
			e := logs[i]
			col := t.TextMuted
			switch e.Level {
			case controller.Good:
				col = t.Success
			case controller.Warn:
				col = t.Warning
			case controller.Bad:
				col = t.Danger
			}
			ui.Row(c.Key(i)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, e.At.Format("15:04:05")).FontSize(12).TextColor(t.TextMuted).FontFeatures("tnum")
				ui.Box(c).Size(6, 6).Radius(3).Background(col)
				ui.Text(c, e.Text).FontSize(12).Grow(1).Wrap().Selectable()
			})
		}
	})
}

func (a *app) settingsCard(c *ui.Context) {
	t := c.Theme()
	card(c, "虚拟屏设置", func() {
		names := make([]string, len(device.Presets))
		for i, p := range device.Presets {
			names[i] = p.Name
		}
		ui.Field(c, "iPad 型号", func() {
			if ui.Select(c, &a.presetName, names).Changed() {
				for _, p := range device.Presets {
					if p.Name == a.presetName {
						a.edit.Device.PresetID = p.ID
						if p.MaxHz < a.edit.Device.RefreshHz {
							a.edit.Device.RefreshHz = p.MaxHz
						}
					}
				}
				a.apply()
			}
		})

		orient := 0
		if a.edit.Device.Portrait {
			orient = 1
		}
		ui.Field(c, "方向", func() {
			if ui.Segmented(c, &orient, orientLabels...).FillEqually().Changed() {
				a.edit.Device.Portrait = orient == 1
				a.apply()
			}
		})

		scale := indexOf(scaleKeys, a.edit.Device.Scale)
		ui.Field(c, "缩放", func() {
			if ui.Segmented(c, &scale, scaleLabels...).FillEqually().Changed() {
				a.edit.Device.Scale = scaleKeys[scale]
				a.apply()
			}
		}).Description("“默认”与 iPad 本身的 Retina 比例一致；“缩小”能放下更多内容；“原生 1:1”文字会非常小。")

		hz := indexOf(hzValues, a.edit.Device.RefreshHz)
		maxHz := device.Find(a.edit.Device.PresetID).MaxHz
		ui.Field(c, "刷新率", func() {
			if ui.Segmented(c, &hz, "60 Hz", "120 Hz").FillEqually().Disabled(maxHz < 120).Changed() {
				a.edit.Device.RefreshHz = hzValues[hz]
				a.apply()
			}
		})

		lay := indexOf(layoutKeys, a.edit.Layout)
		ui.Field(c, "显示方式", func() {
			if ui.Segmented(c, &lay, layoutLabels...).FillEqually().Changed() {
				a.edit.Layout = layoutKeys[lay]
				a.apply()
			}
		}).Description(layoutHelp[lay])

		mode := 0
		if a.edit.Mode == config.ModeManual {
			mode = 1
		}
		ui.Field(c, "创建时机", func() {
			if ui.Segmented(c, &mode, "UU 远程连接时", "仅手动").FillEqually().Changed() {
				a.edit.Mode = config.ModeAuto
				if mode == 1 {
					a.edit.Mode = config.ModeManual
				}
				a.apply()
			}
		})

		ui.Divider(c)
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "登录时启动").FontSize(13)
				ui.Text(c, "在菜单栏后台运行，随时待命").FontSize(11).TextColor(t.TextMuted)
			})
			if ui.Switch(c, &a.openAtLogin).Label("登录时启动").Changed() {
				if err := mygo.App.SetOpenAtLogin(a.openAtLogin); err != nil {
					a.openAtLogin = !a.openAtLogin
					c.Toast("无法设置登录项：" + err.Error())
				}
			}
		})
	})
}

func (a *app) detectCard(c *ui.Context) {
	t := c.Theme()
	if !a.procInit {
		a.procText = strings.Join(a.edit.Processes, ", ")
		a.procInit = true
	}
	card(c, "", func() {
		ui.Collapsible(c, "会话检测参数", &a.advanced, func() {
			ui.Column(c).Gap(12).Padding(8, 0, 0, 0).Children(func() {
				ui.Text(c, "大多数情况下无需修改。可对照左侧流量图，按实际连接时的上行速率调整。").
					FontSize(11).TextColor(t.TextMuted).Wrap()
				d := &a.edit.Detect
				connect, hold := d.ConnectKBps, d.HoldKBps
				secs, wait := float64(d.ConnectSeconds), float64(d.DisconnectSeconds)
				ui.Field(c, "开始阈值（KB/s）", func() {
					if ui.NumberInput(c, &connect, 10, 10000, 10).Changed() {
						d.ConnectKBps = connect
						a.apply()
					}
				}).Description("上行持续高于此值视为有人连入")
				ui.Field(c, "持续秒数", func() {
					if ui.NumberInput(c, &secs, 1, 30, 1).Changed() {
						d.ConnectSeconds = int(secs)
						a.apply()
					}
				})
				ui.Field(c, "保持阈值（KB/s）", func() {
					if ui.NumberInput(c, &hold, 0, 1000, 1).Changed() {
						d.HoldKBps = hold
						a.apply()
					}
				}).Description("画面静止时 UU 仍会保持少量上行")
				ui.Field(c, "断开等待（秒）", func() {
					if ui.NumberInput(c, &wait, 3, 600, 5).Changed() {
						d.DisconnectSeconds = int(wait)
						a.apply()
					}
				})
				ui.Field(c, "监视的进程名（前缀，逗号分隔）", func() {
					if ui.TextInput(c, &a.procText).Placeholder("UURemote").Changed() {
						var procs []string
						for _, p := range strings.Split(a.procText, ",") {
							if p = strings.TrimSpace(p); p != "" {
								procs = append(procs, p)
							}
						}
						a.edit.Processes = procs
						a.apply()
					}
				})
				if ui.Button(c, "恢复默认").Clicked() {
					def := config.Default()
					a.edit.Detect, a.edit.Processes = def.Detect, def.Processes
					a.procText = strings.Join(def.Processes, ", ")
					a.apply()
				}
			})
		})
	})
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
