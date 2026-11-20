package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/display"
	"github.com/Infinity4B/infiwindow/internal/hidpi"
)

var testMonitor = monitorKey{0x5e3, 0x2703}

// withMonitor gives the test app's external monitor the HiDPI sizes macOS
// offers and an installed override (none when installed is nil).
func withMonitor(a *app, installed []hidpi.Size, offered []display.ScaledSize) {
	a.displays[1].Scaled = offered
	a.hidpi.overrides = map[monitorKey]hidpi.Override{}
	if installed != nil {
		a.hidpi.overrides[testMonitor] = hidpi.Override{Exists: true, Managed: true, Sizes: installed}
	}
}

func TestHiDPIActive(t *testing.T) {
	a := newTestApp(pageHiDPI)
	sizes := []hidpi.Size{{W: 2304, H: 1296}, {W: 2048, H: 1152}, {W: 1920, H: 1080}, {W: 1708, H: 960}, {W: 1600, H: 900}, {W: 1464, H: 824}}
	var offered []display.ScaledSize
	for _, s := range sizes {
		offered = append(offered, display.ScaledSize{W: s.W, H: s.H})
	}
	offered = append(offered, display.ScaledSize{W: 1280, H: 720})
	withMonitor(a, sizes, offered)
	tt := ui.NewTester(a.view, testW, 1000)
	expectTexts(t, tt, "已生效", "1920×1080（133%）", "原生 1:1", "200%", "更新", "移除")
	snapshot(t, tt, "hidpi-active")
	if err := tt.Click("1464×824（175%）"); err != nil {
		t.Fatal(err)
	}
	if a.hidpi.sel[testMonitor][hidpi.Size{W: 1464, H: 824}] {
		t.Fatal("175% still selected after unticking")
	}
	a.page = pageOverview
	tt.Frame()
	expectTexts(t, tt, "1 台显示器已开启 HiDPI", "HiDPI")
}

func TestHiDPIPendingAndFresh(t *testing.T) {
	a := newTestApp(pageHiDPI)
	withMonitor(a, []hidpi.Size{{W: 1920, H: 1080}}, []display.ScaledSize{{W: 1280, H: 720}})
	tt := ui.NewTester(a.view, testW, testH)
	expectTexts(t, tt, "待重新插拔")

	b := newTestApp(pageHiDPI)
	withMonitor(b, nil, []display.ScaledSize{{W: 1280, H: 720}})
	tt = ui.NewTester(b.view, testW, testH)
	expectTexts(t, tt, "未安装", "安装")
	snapshot(t, tt, "hidpi-fresh")
}

func TestHiDPIWithoutExternal(t *testing.T) {
	a := newTestApp(pageHiDPI)
	a.displays = a.displays[:1]
	tt := ui.NewTester(a.view, testW, testH)
	expectTexts(t, tt, "没有连接外接显示器")
	if tt.HasText("未安装") || tt.HasText("切换分辨率") {
		t.Fatal("monitor controls shown without an external monitor")
	}
}
