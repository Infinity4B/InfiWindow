// Package controller ties the session detector to the virtual display: it
// creates the display when a remote session starts and removes it when the
// session ends. One goroutine owns all state; the UI talks to it through
// methods that queue commands and receives immutable snapshots.
package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/Infinity4B/infiwindow/internal/config"
	"github.com/Infinity4B/infiwindow/internal/detector"
	"github.com/Infinity4B/infiwindow/internal/device"
	"github.com/Infinity4B/infiwindow/internal/vdisplay"
)

// Displays is what the controller needs from vdisplay.Manager.
type Displays interface {
	Create(spec device.Spec, layout vdisplay.Layout) (uint32, error)
	Destroy()
	Close()
	Active() (uint32, bool)
	ListDisplays() []vdisplay.Display
	TrackModes(idle bool)
	AdoptModes()
}

// Source produces traffic samples, as detector.Watcher does.
type Source interface {
	Run(ctx context.Context, samples chan<- detector.Sample, errs chan<- error)
	SetPrefixes(prefixes []string)
}

// Level of a log entry.
type Level int

const (
	Info Level = iota
	Good
	Warn
	Bad
)

// LogEntry is one line of the activity log.
type LogEntry struct {
	At    time.Time
	Level Level
	Text  string
}

// HistoryLen is how many upload samples (seconds) the snapshot keeps.
const HistoryLen = 90

const maxLogs = 200

// Snapshot is the state the UI shows. It is never modified after publishing.
type Snapshot struct {
	Config    config.Config
	Spec      device.Spec
	SpecErr   string
	Connected bool // a remote session is detected
	Manual    bool // the user asked for the display
	// Suppressed: the user removed the display during a session; it stays
	// off until that session ends.
	Suppressed bool
	Sample     detector.Sample
	History    []float64
	WatchErr   string
	DisplayOn  bool
	DisplayID  uint32
	Busy       bool
	LastErr    string
	Logs       []LogEntry
	Displays   []vdisplay.Display
	Since      time.Time // when the display was created
}

// Controller runs the detect → create/remove loop.
type Controller struct {
	displays Displays
	source   Source
	save     func(config.Config) error
	publish  func(Snapshot)
	now      func() time.Time

	cmds chan func()
	done chan struct{}

	// Owned by the Run goroutine.
	cfg        config.Config
	machine    *detector.Machine
	snap       Snapshot
	history    []float64
	logs       []LogEntry
	retryAfter time.Time
	builtSpec  device.Spec
	builtLay   vdisplay.Layout
}

// New makes a controller. publish is called from the controller goroutine
// with every new snapshot; save persists configuration changes.
func New(cfg config.Config, displays Displays, source Source, save func(config.Config) error, publish func(Snapshot)) *Controller {
	cfg = cfg.Normalize()
	c := &Controller{
		displays: displays,
		source:   source,
		save:     save,
		publish:  publish,
		now:      time.Now,
		cmds:     make(chan func(), 16),
		done:     make(chan struct{}),
		cfg:      cfg,
		machine:  detector.NewMachine(cfg.Detect),
	}
	c.snap.Sample.Conns = -1
	source.SetPrefixes(cfg.Processes)
	return c
}

// Run blocks until ctx is done, then removes the display.
func (c *Controller) Run(ctx context.Context) {
	defer close(c.done)
	samples := make(chan detector.Sample, 4)
	errs := make(chan error, 1)
	go c.source.Run(ctx, samples, errs)

	c.logf(Info, "InfiWindow 已启动，%s", c.modeText())
	c.refreshDisplays()
	c.displays.TrackModes(c.idle())
	c.emit()

	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			if c.snap.DisplayOn {
				c.displays.Close()
			}
			return
		case s := <-samples:
			c.onSample(s)
		case err := <-errs:
			msg := "无法读取网络流量: " + err.Error()
			if c.snap.WatchErr != msg {
				c.logf(Bad, "%s", msg)
			}
			c.snap.WatchErr = msg
		case fn := <-c.cmds:
			fn()
		case <-tick.C:
			c.checkDisplay()
			c.refreshDisplays()
		}
		c.displays.TrackModes(c.idle())
		c.reconcile()
		c.emit()
	}
}

// Done is closed once Run has returned.
func (c *Controller) Done() <-chan struct{} { return c.done }

func (c *Controller) send(fn func()) {
	select {
	case c.cmds <- fn:
	case <-c.done:
	}
}

// SetConfig applies and saves a new configuration.
func (c *Controller) SetConfig(cfg config.Config) {
	c.send(func() {
		cfg = cfg.Normalize()
		old := c.cfg
		c.cfg = cfg
		c.machine.SetParams(cfg.Detect)
		c.source.SetPrefixes(cfg.Processes)
		if old.Mode != cfg.Mode {
			c.logf(Info, "切换为%s", c.modeText())
		}
		if err := c.save(cfg); err != nil {
			c.logf(Bad, "保存设置失败: %v", err)
		}
		c.retryAfter = time.Time{}
	})
}

// CreateNow turns the display on until RemoveNow, whatever the detector says.
func (c *Controller) CreateNow() {
	c.send(func() {
		c.snap.Manual = true
		c.snap.Suppressed = false
		c.retryAfter = time.Time{}
	})
}

// RemoveNow turns the display off. During a detected session it stays off
// until the session ends.
func (c *Controller) RemoveNow() {
	c.send(func() {
		c.snap.Manual = false
		if c.machine.Connected() {
			c.snap.Suppressed = true
		}
	})
}

// Recreate rebuilds the display, e.g. after the user changed resolution in
// System Settings and wants the configured one back.
func (c *Controller) Recreate() {
	c.send(func() {
		c.builtSpec = device.Spec{}
		c.retryAfter = time.Time{}
	})
}

// ClearLog empties the activity log.
func (c *Controller) ClearLog() {
	c.send(func() { c.logs = nil })
}

func (c *Controller) modeText() string {
	if c.cfg.Mode == config.ModeAuto {
		return "自动模式：检测到 UU 远程连接时创建虚拟屏"
	}
	return "手动模式：仅通过按钮创建虚拟屏"
}

func (c *Controller) onSample(s detector.Sample) {
	c.snap.Sample = s
	c.snap.WatchErr = ""
	c.history = append(c.history, s.UploadKBps)
	if len(c.history) > HistoryLen {
		c.history = c.history[len(c.history)-HistoryLen:]
	}
	if c.machine.Step(s.At, s.UploadKBps, s.Conns) {
		if c.machine.Connected() {
			c.logf(Good, "检测到远程会话（上行 %.0f KB/s）", s.UploadKBps)
		} else {
			c.logf(Info, "远程会话已结束")
			c.snap.Suppressed = false
		}
	}
	c.snap.Connected = c.machine.Connected()
}

// idle says no remote session is running or starting, so the display modes
// are the user's own: remote desktop apps change them as they connect.
func (c *Controller) idle() bool {
	return !c.snap.DisplayOn && !c.machine.Connected() &&
		c.snap.Sample.UploadKBps < c.cfg.Detect.ConnectKBps
}

func (c *Controller) desired() bool {
	if c.snap.Manual {
		return true
	}
	return c.cfg.Mode == config.ModeAuto && c.snap.Connected && !c.snap.Suppressed
}

func (c *Controller) reconcile() {
	spec, err := device.BuildSpec(c.cfg.Device)
	c.snap.Spec = spec
	c.snap.SpecErr = ""
	if err != nil {
		c.snap.SpecErr = err.Error()
	}
	want := c.desired() && err == nil

	switch {
	case want && c.snap.DisplayOn && (!specEqual(spec, c.builtSpec) || c.builtLay != c.cfg.Layout):
		c.logf(Info, "设置已变更，重建虚拟屏")
		c.create(spec)
	case want && !c.snap.DisplayOn:
		if c.now().Before(c.retryAfter) {
			return
		}
		c.create(spec)
	case !want && c.snap.DisplayOn:
		c.destroy()
	}
}

func (c *Controller) create(spec device.Spec) {
	if c.idle() {
		// Turned on by hand outside a session: the user may have changed a
		// resolution moments ago, so take the current modes as theirs.
		c.displays.AdoptModes()
	}
	c.snap.Busy = true
	c.emit()
	defer func() { c.snap.Busy = false }()

	id, err := c.displays.Create(spec, c.cfg.Layout)
	c.builtSpec, c.builtLay = spec, c.cfg.Layout
	if id == 0 {
		c.snap.DisplayOn, c.snap.DisplayID = false, 0
		c.snap.LastErr = err.Error()
		c.retryAfter = c.now().Add(15 * time.Second)
		c.logf(Bad, "创建虚拟屏失败: %v（15 秒后重试）", err)
		c.refreshDisplays()
		return
	}
	c.snap.DisplayOn, c.snap.DisplayID = true, id
	c.snap.Since = c.now()
	c.snap.LastErr = ""
	if err != nil {
		c.snap.LastErr = err.Error()
		c.logf(Warn, "%v", err)
	}
	w, h := spec.Selected.PixelSize()
	c.logf(Good, "虚拟屏已创建：%d×%d pt（%d×%d px）@%gHz，%s",
		spec.Selected.Width, spec.Selected.Height, w, h, spec.RefreshHz, layoutText(c.cfg.Layout))
	c.refreshDisplays()
}

func (c *Controller) destroy() {
	c.snap.Busy = true
	c.emit()
	c.displays.Destroy()
	c.snap.Busy = false
	c.snap.DisplayOn, c.snap.DisplayID = false, 0
	c.builtSpec = device.Spec{}
	c.logf(Info, "虚拟屏已移除，显示器排列已恢复")
	c.refreshDisplays()
}

// checkDisplay notices a display that macOS removed behind our back.
func (c *Controller) checkDisplay() {
	if !c.snap.DisplayOn {
		return
	}
	if _, ok := c.displays.Active(); !ok {
		c.snap.DisplayOn, c.snap.DisplayID = false, 0
		c.builtSpec = device.Spec{}
		c.logf(Warn, "虚拟屏被系统移除")
	}
}

func (c *Controller) refreshDisplays() {
	c.snap.Displays = c.displays.ListDisplays()
}

func (c *Controller) logf(level Level, format string, args ...any) {
	c.logs = append(c.logs, LogEntry{At: c.now(), Level: level, Text: fmt.Sprintf(format, args...)})
	if len(c.logs) > maxLogs {
		c.logs = c.logs[len(c.logs)-maxLogs:]
	}
}

func (c *Controller) emit() {
	s := c.snap
	s.Config = c.cfg
	s.History = append([]float64(nil), c.history...)
	s.Logs = append([]LogEntry(nil), c.logs...)
	s.Displays = append([]vdisplay.Display(nil), c.snap.Displays...)
	c.publish(s)
}

func layoutText(l vdisplay.Layout) string {
	switch l {
	case vdisplay.LayoutMirror:
		return "其他屏幕镜像虚拟屏"
	case vdisplay.LayoutMain:
		return "设为主显示器"
	default:
		return "扩展显示"
	}
}

func specEqual(a, b device.Spec) bool {
	if a.Name != b.Name || a.Selected != b.Selected || a.RefreshHz != b.RefreshHz ||
		a.MaxPixelW != b.MaxPixelW || a.MaxPixelH != b.MaxPixelH || len(a.Modes) != len(b.Modes) {
		return false
	}
	for i := range a.Modes {
		if a.Modes[i] != b.Modes[i] {
			return false
		}
	}
	return true
}
