package main

import (
	"errors"
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/activity"
	"github.com/Infinity4B/infiwindow/internal/display"
	"github.com/Infinity4B/infiwindow/internal/hidpi"
)

// monitorKey identifies a monitor model: overrides apply per vendor/product.
type monitorKey struct{ vendor, product uint32 }

func keyOfDisplay(d display.Display) monitorKey { return monitorKey{d.VendorID, d.ProductID} }

// hidpiState is the UI state of the external-monitor HiDPI page.
type hidpiState struct {
	overrides map[monitorKey]hidpi.Override
	readErr   map[monitorKey]string
	// sel holds the ticked resolutions per monitor, seeded from the
	// installed override.
	sel  map[monitorKey]map[hidpi.Size]bool
	busy bool
	note string
	bad  bool
}

func externalMonitors(ds []display.Display) []display.Display {
	var out []display.Display
	for _, d := range ds {
		if d.VendorID != 0 && d.NativeW > 0 {
			out = append(out, d)
		}
	}
	return out
}

// refreshOverrides reads the installed override of every external monitor.
func (a *app) refreshOverrides() {
	h := &a.hidpi
	h.overrides = map[monitorKey]hidpi.Override{}
	h.readErr = map[monitorKey]string{}
	for _, d := range externalMonitors(a.displays) {
		k := keyOfDisplay(d)
		o, err := hidpi.Read(d.VendorID, d.ProductID)
		if err != nil {
			h.readErr[k] = err.Error()
		}
		h.overrides[k] = o
	}
}

// selection returns the ticked resolutions of a monitor, seeding them with
// the installed ones, or every option when nothing is installed yet.
func (a *app) selection(k monitorKey, o hidpi.Override, opts []hidpi.Option) map[hidpi.Size]bool {
	h := &a.hidpi
	if h.sel == nil {
		h.sel = map[monitorKey]map[hidpi.Size]bool{}
	}
	if s, ok := h.sel[k]; ok {
		return s
	}
	s := map[hidpi.Size]bool{}
	for _, opt := range opts {
		s[opt.Size] = !o.Exists || slices.Contains(o.Sizes, opt.Size)
	}
	h.sel[k] = s
	return s
}

// hidpiActive counts the external monitors whose InfiWindow override is
// installed and in effect.
func (a *app) hidpiActive() int {
	n := 0
	for _, d := range externalMonitors(a.displays) {
		if o := a.hidpi.overrides[keyOfDisplay(d)]; o.Managed && len(d.Scaled) > 0 && !a.overridePending(d, o) {
			n++
		}
	}
	return n
}

// overridePending says the installed override lists sizes macOS does not
// offer yet: it reads the file when the monitor connects.
func (a *app) overridePending(d display.Display, o hidpi.Override) bool {
	for _, s := range o.Sizes {
		if !slices.Contains(d.Scaled, display.ScaledSize{W: s.W, H: s.H}) {
			return true
		}
	}
	return false
}

// hidpiSummary is the feature's state in a few words, for the overview.
func (a *app) hidpiSummary(t *ui.Theme) (string, ui.Color) {
	ext := externalMonitors(a.displays)
	switch n := a.hidpiActive(); {
	case len(ext) == 0:
		return "没有连接外接显示器", t.TextMuted
	case n > 0:
		return fmt.Sprintf("%d 台显示器已开启 HiDPI", n), t.Success
	}
	for _, d := range ext {
		if o := a.hidpi.overrides[keyOfDisplay(d)]; o.Managed {
			return "已安装，待重新插拔显示器", t.Warning
		}
	}
	return "未安装", t.TextMuted
}

func (a *app) hidpiPage(c *ui.Context) {
	t := c.Theme()
	ext := externalMonitors(a.displays)
	if len(ext) == 0 {
		card(c, "", func() {
			ui.Text(c, "没有连接外接显示器").FontSize(15).Bold()
			ui.Text(c, "接上外接显示器后，这里会列出它支持的 HiDPI 缩放档位。").FontSize(12).TextColor(t.TextMuted).Wrap()
		})
	}
	if a.hidpi.note != "" {
		col := t.TextMuted
		if a.hidpi.bad {
			col = t.Danger
		}
		ui.Text(c, a.hidpi.note).FontSize(12).TextColor(col).Wrap().Selectable()
	}
	for _, d := range ext {
		card(c.Key(d.ID), "", func() { a.hidpiMonitor(c, d) })
	}
	card(c, "工作原理", func() {
		ui.Text(c, "macOS 会读取 /Library/Displays 下的显示器覆盖文件，为这台显示器加上其中列出的 HiDPI 分辨率："+
			"按 2 倍像素渲染，再缩小到屏幕的原生分辨率。安装时需要管理员密码，重新插拔显示器后生效，之后不需要 InfiWindow 运行。"+
			"150% 这类非整数档位会略微发软；200% 像素一一对应，最锐利，macOS 本来就提供。").
			FontSize(12).TextColor(t.TextMuted).Wrap()
	})
}

func (a *app) hidpiMonitor(c *ui.Context, d display.Display) {
	t := c.Theme()
	k := keyOfDisplay(d)
	o := a.hidpi.overrides[k]
	opts := hidpi.Options(d.NativeW, d.NativeH)
	offered := func(s hidpi.Size) bool {
		return slices.Contains(d.Scaled, display.ScaledSize{W: s.W, H: s.H})
	}
	pending := a.overridePending(d, o)

	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, fmt.Sprintf("外接显示器 #%d", d.ID)).FontSize(13).Bold()
			ui.Text(c, fmt.Sprintf("原生 %d×%d · 厂商 %04X · 型号 %04X", d.NativeW, d.NativeH, d.VendorID, d.ProductID)).
				FontSize(12).TextColor(t.TextMuted)
		})
		switch {
		case !o.Exists:
			ui.Badge(c, "未安装")
		case !o.Managed:
			ui.Badge(c, "其他工具的覆盖文件").Background(t.Warning.Alpha(0.15)).TextColor(t.Warning)
		case pending:
			ui.Badge(c, "待重新插拔").Background(t.Warning.Alpha(0.15)).TextColor(t.Warning)
		default:
			ui.Badge(c, "已生效").Background(t.Success.Alpha(0.15)).TextColor(t.Success)
		}
	})
	if msg := a.hidpi.readErr[k]; msg != "" {
		ui.Text(c, "读取覆盖文件失败："+msg).FontSize(12).TextColor(t.Danger).Wrap()
	}
	if o.Exists && !o.Managed {
		ui.Text(c, hidpi.ErrForeign.Error()+"："+hidpi.Path(d.VendorID, d.ProductID)).
			FontSize(12).TextColor(t.TextMuted).Wrap().Selectable()
		return
	}
	if pending {
		ui.Text(c, "覆盖文件已更新。拔下显示器的线再插回（或重启 Mac）后，macOS 才会读取新的分辨率。").
			FontSize(12).TextColor(t.Warning).Wrap()
	}

	sel := a.selection(k, o, opts)
	ui.Row(c).GapX(16).GapY(4).Wrap().Children(func() {
		for _, opt := range opts {
			on := sel[opt.Size]
			if ui.Checkbox(c.Key(opt.Size), &on, fmt.Sprintf("%s（%d%%）", opt.Size, opt.Percent)).
				Disabled(a.hidpi.busy).Changed() {
				sel[opt.Size] = on
			}
		}
	})
	var chosen []hidpi.Size
	for _, opt := range opts {
		if sel[opt.Size] {
			chosen = append(chosen, opt.Size)
		}
	}
	ui.Row(c).Gap(8).Children(func() {
		switch {
		case !o.Exists:
			if ui.PrimaryButton(c, "安装").Disabled(a.hidpi.busy || len(chosen) == 0).Clicked() {
				a.runHiDPI(d, "已安装 HiDPI 分辨率，拔下显示器的线再插回后生效", func() error {
					return hidpi.Install(d.VendorID, d.ProductID, chosen)
				})
			}
		default:
			if ui.PrimaryButton(c, "更新").Disabled(a.hidpi.busy || len(chosen) == 0 || slices.Equal(chosen, o.Sizes)).Clicked() {
				a.runHiDPI(d, "已更新 HiDPI 分辨率，拔下显示器的线再插回后生效", func() error {
					return hidpi.Install(d.VendorID, d.ProductID, chosen)
				})
			}
			if ui.Button(c, "移除").Disabled(a.hidpi.busy).
				Tooltip("删除覆盖文件。正在用的 HiDPI 分辨率会在重新插拔后消失").Clicked() {
				a.runHiDPI(d, "已移除 HiDPI 覆盖文件，重新插拔显示器后恢复原样", func() error {
					return hidpi.Remove(d.VendorID, d.ProductID)
				})
			}
		}
	})

	// Switching: every scaled size macOS offers that InfiWindow lists, the
	// built-in 2x of the native size, and native 1:1.
	type choice struct {
		label string
		w, h  int
		hi    bool
	}
	var choices []choice
	for _, opt := range opts {
		if offered(opt.Size) {
			choices = append(choices, choice{fmt.Sprintf("%d%%", opt.Percent), opt.W, opt.H, true})
		}
	}
	if half := (hidpi.Size{W: d.NativeW / 2, H: d.NativeH / 2}); offered(half) {
		choices = append(choices, choice{"200%", half.W, half.H, true})
	}
	if len(choices) == 0 {
		return
	}
	choices = append([]choice{{"原生 1:1", d.NativeW, d.NativeH, false}}, choices...)
	locked := a.snap.DisplayOn || a.snap.Busy
	ui.Field(c, "切换分辨率", func() {
		ui.Row(c).Gap(6).Wrap().Children(func() {
			for _, ch := range choices {
				current := d.Width == ch.w && d.Height == ch.h && d.HiDPI() == ch.hi
				label := ch.label
				tip := fmt.Sprintf("看起来像 %d×%d", ch.w, ch.h)
				var b ui.Element
				if current {
					b = ui.PrimaryButton(c.Key(label), label)
				} else {
					b = ui.Button(c.Key(label), label)
				}
				if b.Disabled(locked || a.hidpi.busy).Tooltip(tip).Clicked() && !current {
					a.runHiDPI(d, fmt.Sprintf("外接显示器 #%d 切换为 %s（看起来像 %d×%d）", d.ID, ch.label, ch.w, ch.h), func() error {
						return display.SetMode(d.ID, ch.w, ch.h, ch.hi)
					})
				}
			}
		})
	}).Description(func() string {
		if locked {
			return "虚拟屏运行中，移除后再切换。"
		}
		return fmt.Sprintf("当前看起来像 %d×%d。也可以在“系统设置 → 显示器”里切换。", d.Width, d.Height)
	}())
}

// runHiDPI runs fn off the main thread: the password dialog and display
// reconfiguration both block.
func (a *app) runHiDPI(d display.Display, done string, fn func() error) {
	a.hidpi.busy, a.hidpi.note, a.hidpi.bad = true, "", false
	go func() {
		err := fn()
		update := func() {
			a.hidpi.busy = false
			switch {
			case errors.Is(err, hidpi.ErrCanceled):
			case err != nil:
				a.hidpi.note, a.hidpi.bad = err.Error(), true
				a.log.Add(hidpiSource, activity.Bad, "%s", err.Error())
			default:
				a.hidpi.note = done
				a.log.Add(hidpiSource, activity.Good, "%s", done)
			}
			// Reseed the ticks from what is installed now.
			delete(a.hidpi.sel, keyOfDisplay(d))
			a.refreshOverrides()
			a.refreshDisplays()
		}
		if a.win != nil {
			a.win.Update(update)
		} else {
			update()
		}
	}()
}
