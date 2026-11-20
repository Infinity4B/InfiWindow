package main

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/config"
	"github.com/Infinity4B/infiwindow/internal/controller"
	"github.com/Infinity4B/infiwindow/internal/detector"
	"github.com/Infinity4B/infiwindow/internal/device"
	"github.com/Infinity4B/infiwindow/internal/vdisplay"
)

type nopDisplays struct{}

func (nopDisplays) Create(device.Spec, vdisplay.Layout) (uint32, error) { return 1, nil }
func (nopDisplays) Destroy()                                            {}
func (nopDisplays) Close()                                              {}
func (nopDisplays) Active() (uint32, bool)                              { return 0, false }
func (nopDisplays) ListDisplays() []vdisplay.Display                    { return nil }
func (nopDisplays) TrackModes(bool)                                     {}
func (nopDisplays) AdoptModes()                                         {}

type nopSource struct{}

func (nopSource) Run(context.Context, chan<- detector.Sample, chan<- error) {}
func (nopSource) SetPrefixes([]string)                                      {}

func newTestApp() *app {
	cfg := config.Default()
	a := &app{edit: cfg}
	a.syncPresetName()
	// The controller is not running: commands queue up harmlessly.
	a.ctl = controller.New(cfg, nopDisplays{}, nopSource{}, func(config.Config) error { return nil }, func(controller.Snapshot) {})
	return a
}

func activeSnapshot(cfg config.Config) controller.Snapshot {
	spec, _ := device.BuildSpec(cfg.Device)
	hist := make([]float64, 0, 90)
	for i := range 90 {
		v := 2.0
		if i > 40 {
			v = 900 + float64(i%7)*120
		}
		hist = append(hist, v)
	}
	now := time.Now()
	return controller.Snapshot{
		Config: cfg, Spec: spec, Connected: true, DisplayOn: true, DisplayID: 9,
		Since:   now.Add(-3*time.Minute - 12*time.Second),
		Sample:  detector.Sample{UploadKBps: 1240, DownKBps: 36, Processes: 2, Conns: 4},
		History: hist,
		Logs: []controller.LogEntry{
			{At: now.Add(-200 * time.Second), Level: controller.Info, Text: "InfiWindow 已启动，自动模式：检测到 UU 远程连接时创建虚拟屏"},
			{At: now.Add(-193 * time.Second), Level: controller.Good, Text: "检测到远程会话（上行 860 KB/s）"},
			{At: now.Add(-192 * time.Second), Level: controller.Good, Text: "虚拟屏已创建：1376×1032 pt（2752×2064 px）@60Hz，其他屏幕镜像虚拟屏"},
		},
		Displays: []vdisplay.Display{
			{ID: 9, Width: 1376, Height: 1032, PixelWidth: 2752, PixelHeight: 2064, RefreshHz: 60, Main: true, Virtual: true},
			{ID: 1, Width: 1376, Height: 1032, PixelWidth: 2752, PixelHeight: 2064, RefreshHz: 120, Builtin: true, MirrorOf: 9},
		},
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

func TestWaitingState(t *testing.T) {
	a := newTestApp()
	a.hasSnap = true
	a.snap = controller.Snapshot{Config: a.edit, Spec: must(device.BuildSpec(a.edit.Device))}
	a.snap.Sample.Conns = -1
	tt := ui.NewTester(a.view, 980, 720)
	for _, want := range []string{"等待 UU 远程连接", "1376×1032 pt", "2752×2064 px", `iPad Pro 13" (M4)`, "立即创建虚拟屏"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "waiting-light")
	tt.SetDark(true)
	tt.Frame()
	snapshot(t, tt, "waiting-dark")
}

func TestActiveState(t *testing.T) {
	a := newTestApp()
	a.hasSnap = true
	a.snap = activeSnapshot(a.edit)
	tt := ui.NewTester(a.view, 980, 720)
	for _, want := range []string{"虚拟屏运行中", "移除虚拟屏", "InfiWindow 虚拟屏", "1.2 MB/s"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q", want)
		}
	}
	snapshot(t, tt, "active-light")
}

func TestSettingsEditConfig(t *testing.T) {
	a := newTestApp()
	a.hasSnap = true
	a.snap = controller.Snapshot{Config: a.edit, Spec: must(device.BuildSpec(a.edit.Device))}
	tt := ui.NewTester(a.view, 980, 720)
	if err := tt.Click("竖屏"); err != nil {
		t.Fatal(err)
	}
	if !a.edit.Device.Portrait {
		t.Fatal("portrait not applied")
	}
	if err := tt.Click("主显示器"); err != nil {
		t.Fatal(err)
	}
	if a.edit.Layout != vdisplay.LayoutMain {
		t.Fatalf("layout = %q", a.edit.Layout)
	}
	if err := tt.Click("仅手动"); err != nil {
		t.Fatal(err)
	}
	if a.edit.Mode != config.ModeManual || !tt.HasText("手动模式") {
		t.Fatal("manual mode not applied")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
