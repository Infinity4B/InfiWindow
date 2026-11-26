package main

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/activity"
	"github.com/Infinity4B/infiwindow/internal/display"
	"github.com/Infinity4B/infiwindow/internal/remote"
	"github.com/Infinity4B/infiwindow/internal/remote/detector"
	"github.com/Infinity4B/infiwindow/internal/remote/device"
)

type nopDisplays struct{}

func (nopDisplays) Create(device.Spec, display.Layout) (uint32, error) { return 1, nil }
func (nopDisplays) Destroy()                                           {}
func (nopDisplays) Close()                                             {}
func (nopDisplays) Active() (uint32, bool)                             { return 0, false }
func (nopDisplays) TrackModes(bool)                                    {}
func (nopDisplays) AdoptModes()                                        {}

type nopSource struct{}

func (nopSource) Run(context.Context, chan<- detector.Sample, chan<- error) {}
func (nopSource) SetPrefixes([]string)                                      {}

const testW, testH = 1080, 900

// newTestApp is an app whose controller is not running: commands queue up
// harmlessly. It starts on page.
func newTestApp(page string) *app {
	a := newApp()
	a.page = page
	a.edit = remote.DefaultConfig()
	a.syncPresetName()
	a.ctl = remote.New(a.edit, nopDisplays{}, nopSource{}, a.log, func(remote.Config) error { return nil }, func(remote.Snapshot) {})
	a.hasSnap = true
	a.snap = remote.Snapshot{Config: a.edit, Spec: must(device.BuildSpec(a.edit.ActiveDevice()))}
	a.snap.Sample.Conns = -1
	a.displays = []display.Display{
		{ID: 1, Width: 1512, Height: 982, PixelWidth: 3024, PixelHeight: 1964, RefreshHz: 120, Main: true, Builtin: true},
		{ID: 5, X: -186, Y: -1080, Width: 1920, Height: 1080, PixelWidth: 3840, PixelHeight: 2160, RefreshHz: 100,
			VendorID: 0x5e3, ProductID: 0x2703, NativeW: 2560, NativeH: 1440},
	}
	return a
}

func activeSnapshot(cfg remote.Config) remote.Snapshot {
	spec, _ := device.BuildSpec(cfg.ActiveDevice())
	hist := make([]float64, 0, 90)
	for i := range 90 {
		v := 2.0
		if i > 40 {
			v = 900 + float64(i%7)*120
		}
		hist = append(hist, v)
	}
	return remote.Snapshot{
		Config: cfg, Spec: spec, Connected: true, DisplayOn: true, DisplayID: 9,
		Since:   time.Now().Add(-3*time.Minute - 12*time.Second),
		Sample:  detector.Sample{UploadKBps: 1240, DownKBps: 36, Processes: 2, Conns: 4},
		History: hist,
	}
}

// snapshot writes the tester's frame when IW_SNAPSHOT_DIR is set, to look at
// the layout without launching the app.
func snapshot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("IW_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func expectTexts(t *testing.T, tt *ui.Tester, want ...string) {
	t.Helper()
	for _, w := range want {
		if !tt.HasText(w) {
			t.Errorf("missing %q in %q", w, tt.Texts())
		}
	}
}

func TestOverview(t *testing.T) {
	a := newTestApp(pageOverview)
	tt := ui.NewTester(a.view, testW, testH)
	expectTexts(t, tt, "InfiWindow", "显示器", "远程扩展屏", "外接屏 HiDPI", "等待远程连接", "未安装",
		"内置显示器", "外接显示器 #5", "主显示器")
	snapshot(t, tt, "overview-light")
	tt.SetDark(true)
	tt.Frame()
	snapshot(t, tt, "overview-dark")
}

func TestSidebarNavigates(t *testing.T) {
	a := newTestApp(pageOverview)
	tt := ui.NewTester(a.view, testW, testH)
	for _, step := range []struct{ label, page, text string }{
		{"远程扩展屏", pageRemote, "立即创建虚拟屏"},
		{"外接屏 HiDPI", pageHiDPI, "工作原理"},
		{"活动记录", pageLog, "暂无记录"},
		{"设置", pageSettings, "登录时启动"},
		{"显示器", pageOverview, "排列"},
	} {
		if err := tt.Click(step.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.page != step.page || !tt.HasText(step.text) {
			t.Fatalf("after clicking %q: page %q, texts %q", step.label, a.page, tt.Texts())
		}
	}
}

func TestRemoteWaiting(t *testing.T) {
	a := newTestApp(pageRemote)
	tt := ui.NewTester(a.view, testW, testH)
	expectTexts(t, tt, "等待远程连接", "1376×1032 pt", "2752×2064 px", `iPad Pro 13" (M4)`, "立即创建虚拟屏", "自动模式")
	snapshot(t, tt, "remote-waiting")
}

func TestRemoteActive(t *testing.T) {
	a := newTestApp(pageRemote)
	a.snap = activeSnapshot(a.edit)
	a.displays = append(a.displays, display.Display{ID: 9, Width: 1376, Height: 1032, PixelWidth: 2752, PixelHeight: 2064, RefreshHz: 60, Virtual: true})
	tt := ui.NewTester(a.view, testW, testH)
	expectTexts(t, tt, "虚拟屏运行中", "移除虚拟屏", "1.2 MB/s")
	snapshot(t, tt, "remote-active")
	a.page = pageOverview
	tt.Frame()
	expectTexts(t, tt, "远程扩展屏（虚拟）", "虚拟")
}

func TestRemoteSettingsEditConfig(t *testing.T) {
	a := newTestApp(pageRemote)
	tt := ui.NewTester(a.view, testW, testH)
	if err := tt.Click("竖屏"); err != nil {
		t.Fatal(err)
	}
	if !a.edit.Profiles.IPad.Portrait {
		t.Fatal("portrait not applied")
	}
	if err := tt.Click("主显示器"); err != nil {
		t.Fatal(err)
	}
	if a.edit.Layout != display.LayoutMain {
		t.Fatalf("layout = %q", a.edit.Layout)
	}
	if err := tt.Click("仅手动"); err != nil {
		t.Fatal(err)
	}
	if a.edit.Mode != remote.ModeManual || !tt.HasText("手动模式") {
		t.Fatal("manual mode not applied")
	}
}

func TestRemoteSwitchClientProfile(t *testing.T) {
	a := newTestApp(pageRemote)
	tt := ui.NewTester(a.view, testW, testH)
	expectTexts(t, tt, "客户端设备", "iPad 型号", "放大", "默认", "缩小", "原生 1:1")

	if err := tt.Click("电脑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.edit.Active != device.KindPC {
		t.Fatalf("active = %q, want pc", a.edit.Active)
	}
	expectTexts(t, tt, "显示器分辨率", "1920×1080 (1080p · 16:9)", "100%", "125%", "150%", "200%")

	if err := tt.Click("150%"); err != nil {
		t.Fatal(err)
	}
	if a.edit.Profiles.PC.Scale != device.Scale150 {
		t.Fatalf("pc scale = %q, want 150", a.edit.Profiles.PC.Scale)
	}

	// Switch back to iPad and verify its profile stayed independent.
	if err := tt.Click("iPad"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.edit.Active != device.KindIPad || a.edit.Profiles.IPad.Scale != device.ScaleDefault || a.edit.Profiles.PC.Scale != device.Scale150 {
		t.Fatalf("profiles not independent: active=%q ipad=%+v pc=%+v", a.edit.Active, a.edit.Profiles.IPad, a.edit.Profiles.PC)
	}
}

func TestLogPage(t *testing.T) {
	a := newTestApp(pageLog)
	a.log.Add(remote.LogSource, activity.Good, "虚拟屏已创建")
	a.log.Add(hidpiSource, activity.Bad, "安装失败")
	tt := ui.NewTester(a.view, testW, testH)
	expectTexts(t, tt, "虚拟屏已创建", "安装失败", remote.LogSource, hidpiSource)
	if err := tt.Click("清空"); err != nil {
		t.Fatal(err)
	}
	if len(a.log.Entries()) != 0 {
		t.Fatal("log not cleared")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
