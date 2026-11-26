// Package remote is the remote display feature: while a remote-desktop app
// (UU Remote by default) streams this Mac, it adds a virtual display sized
// like the remote device. The Controller ties the session detector to the
// virtual display; one goroutine owns all state, the UI talks to it through
// methods that queue commands and receives immutable snapshots.
package remote

import (
	"context"
	"time"

	"github.com/Infinity4B/infiwindow/internal/activity"
	"github.com/Infinity4B/infiwindow/internal/display"
	"github.com/Infinity4B/infiwindow/internal/remote/detector"
	"github.com/Infinity4B/infiwindow/internal/remote/device"
)

// Displays is what the controller needs from display.Manager.
type Displays interface {
	Create(spec device.Spec, layout display.Layout) (uint32, error)
	Destroy()
	Close()
	Active() (uint32, bool)
	TrackModes(idle bool)
	AdoptModes()
}

// Source produces traffic samples, as detector.Watcher does.
type Source interface {
	Run(ctx context.Context, samples chan<- detector.Sample, errs chan<- error)
	SetPrefixes(prefixes []string)
}

// HistoryLen is how many upload samples (seconds) the snapshot keeps.
const HistoryLen = 90

// LogSource tags the entries this feature writes to the activity log.
const LogSource = "远程扩展屏"

// Snapshot is the state the UI shows. It is never modified after publishing.
type Snapshot struct {
	Config    Config
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
	Since      time.Time // when the display was created
}

// Controller runs the detect → create/remove loop.
type Controller struct {
	displays Displays
	source   Source
	log      *activity.Log
	save     func(Config) error
	publish  func(Snapshot)
	now      func() time.Time

	cmds chan func()
	done chan struct{}

	// Owned by the Run goroutine.
	cfg        Config
	machine    *detector.Machine
	snap       Snapshot
	history    []float64
	retryAfter time.Time
	builtSpec  device.Spec
	builtLay   display.Layout
}

// New makes a controller writing to log. publish is called from the controller goroutine
// with every new snapshot; save persists configuration changes.
func New(cfg Config, displays Displays, source Source, log *activity.Log, save func(Config) error, publish func(Snapshot)) *Controller {
	cfg = cfg.Normalize()
	c := &Controller{
		displays: displays,
		source:   source,
		log:      log,
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

	c.logf(activity.Info, "%s", c.modeText())
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
				c.logf(activity.Bad, "%s", msg)
			}
			c.snap.WatchErr = msg
		case fn := <-c.cmds:
			fn()
		case <-tick.C:
			c.checkDisplay()
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
func (c *Controller) SetConfig(cfg Config) {
	c.send(func() {
		cfg = cfg.Normalize()
		old := c.cfg
		c.cfg = cfg
		c.machine.SetParams(cfg.Detect)
		c.source.SetPrefixes(cfg.Processes)
		if old.Active != cfg.Active {
			c.logf(activity.Info, "切换客户端配置为 %s", device.KindLabel(cfg.Active))
		}
		if old.Mode != cfg.Mode {
			c.logf(activity.Info, "切换为%s", c.modeText())
		}
		if err := c.save(cfg); err != nil {
			c.logf(activity.Bad, "保存设置失败: %v", err)
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

func (c *Controller) modeText() string {
	if c.cfg.Mode == ModeAuto {
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
			c.logf(activity.Good, "检测到远程会话（上行 %.0f KB/s）", s.UploadKBps)
		} else {
			c.logf(activity.Info, "远程会话已结束")
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
	return c.cfg.Mode == ModeAuto && c.snap.Connected && !c.snap.Suppressed
}

func (c *Controller) reconcile() {
	spec, err := device.BuildSpec(c.cfg.ActiveDevice())
	c.snap.Spec = spec
	c.snap.SpecErr = ""
	if err != nil {
		c.snap.SpecErr = err.Error()
	}
	want := c.desired() && err == nil

	switch {
	case want && c.snap.DisplayOn && (!specEqual(spec, c.builtSpec) || c.builtLay != c.cfg.Layout):
		c.logf(activity.Info, "设置已变更，重建虚拟屏")
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
		c.logf(activity.Bad, "创建虚拟屏失败: %v（15 秒后重试）", err)
		return
	}
	c.snap.DisplayOn, c.snap.DisplayID = true, id
	c.snap.Since = c.now()
	c.snap.LastErr = ""
	if err != nil {
		c.snap.LastErr = err.Error()
		c.logf(activity.Warn, "%v", err)
	}
	w, h := spec.Selected.PixelSize()
	c.logf(activity.Good, "虚拟屏已创建（%s）：%d×%d pt（%d×%d px）@%gHz，%s",
		device.KindLabel(c.cfg.Active), spec.Selected.Width, spec.Selected.Height, w, h, spec.RefreshHz, layoutText(c.cfg.Layout))
}

func (c *Controller) destroy() {
	c.snap.Busy = true
	c.emit()
	c.displays.Destroy()
	c.snap.Busy = false
	c.snap.DisplayOn, c.snap.DisplayID = false, 0
	c.builtSpec = device.Spec{}
	c.logf(activity.Info, "虚拟屏已移除，显示器排列已恢复")
}

// checkDisplay notices a display that macOS removed behind our back.
func (c *Controller) checkDisplay() {
	if !c.snap.DisplayOn {
		return
	}
	if _, ok := c.displays.Active(); !ok {
		c.snap.DisplayOn, c.snap.DisplayID = false, 0
		c.builtSpec = device.Spec{}
		c.logf(activity.Warn, "虚拟屏被系统移除")
	}
}

func (c *Controller) logf(level activity.Level, format string, args ...any) {
	c.log.Add(LogSource, level, format, args...)
}

func (c *Controller) emit() {
	s := c.snap
	s.Config = c.cfg
	s.History = append([]float64(nil), c.history...)
	c.publish(s)
}

func layoutText(l display.Layout) string {
	switch l {
	case display.LayoutMirror:
		return "其他屏幕镜像虚拟屏"
	case display.LayoutMain:
		return "设为主显示器"
	default:
		return "扩展显示"
	}
}

func specEqual(a, b device.Spec) bool {
	if a.Name != b.Name || a.Selected != b.Selected || a.RefreshHz != b.RefreshHz ||
		a.MaxPixelW != b.MaxPixelW || a.MaxPixelH != b.MaxPixelH ||
		a.WidthMM != b.WidthMM || a.HeightMM != b.HeightMM || len(a.Modes) != len(b.Modes) {
		return false
	}
	for i := range a.Modes {
		if a.Modes[i] != b.Modes[i] {
			return false
		}
	}
	return true
}
