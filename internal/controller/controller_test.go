package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Infinity4B/infiwindow/internal/config"
	"github.com/Infinity4B/infiwindow/internal/detector"
	"github.com/Infinity4B/infiwindow/internal/device"
	"github.com/Infinity4B/infiwindow/internal/vdisplay"
)

type fakeDisplays struct {
	mu      sync.Mutex
	on      bool
	creates int
	fail    error
	last    device.Spec
}

func (f *fakeDisplays) Create(s device.Spec, _ vdisplay.Layout) (uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	if f.fail != nil {
		return 0, f.fail
	}
	f.on, f.last = true, s
	return 42, nil
}
func (f *fakeDisplays) Destroy() { f.mu.Lock(); f.on = false; f.mu.Unlock() }
func (f *fakeDisplays) Close()   { f.Destroy() }
func (f *fakeDisplays) Active() (uint32, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return 42, f.on
}
func (f *fakeDisplays) ListDisplays() []vdisplay.Display { return nil }

type fakeSource struct{ ch chan detector.Sample }

func (s *fakeSource) Run(ctx context.Context, out chan<- detector.Sample, _ chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case v := <-s.ch:
			out <- v
		}
	}
}
func (s *fakeSource) SetPrefixes([]string) {}

type harness struct {
	t     *testing.T
	ctl   *Controller
	disp  *fakeDisplays
	src   *fakeSource
	snaps chan Snapshot
	clock time.Time
}

func newHarness(t *testing.T, mutate func(*config.Config)) *harness {
	cfg := config.Default()
	cfg.Detect = detector.Params{ConnectKBps: 100, ConnectSeconds: 1, HoldKBps: 5, DisconnectSeconds: 3}
	if mutate != nil {
		mutate(&cfg)
	}
	h := &harness{t: t, disp: &fakeDisplays{}, src: &fakeSource{ch: make(chan detector.Sample)},
		snaps: make(chan Snapshot, 256), clock: time.Unix(1000, 0)}
	h.ctl = New(cfg, h.disp, h.src, func(config.Config) error { return nil }, func(s Snapshot) { h.snaps <- s })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); <-h.ctl.Done() })
	go h.ctl.Run(ctx)
	return h
}

func (h *harness) sample(kbps float64) Snapshot {
	h.clock = h.clock.Add(time.Second)
	h.src.ch <- detector.Sample{At: h.clock, UploadKBps: kbps, Conns: -1}
	return h.settle()
}

// settle returns the last snapshot after the controller went quiet.
func (h *harness) settle() Snapshot {
	var last Snapshot
	timeout := time.After(2 * time.Second)
	for {
		select {
		case s := <-h.snaps:
			last = s
		case <-time.After(50 * time.Millisecond):
			return last
		case <-timeout:
			h.t.Fatal("no snapshot")
		}
	}
}

func TestAutoCreatesAndRemoves(t *testing.T) {
	h := newHarness(t, nil)
	h.settle()
	if s := h.sample(500); !s.Connected || !s.DisplayOn {
		t.Fatalf("expected display on: %+v", s)
	}
	if w, _ := h.disp.last.Selected.PixelSize(); w != 2752 {
		t.Fatalf("wrong spec: %+v", h.disp.last.Selected)
	}
	for range 3 {
		h.sample(0)
	}
	if s := h.sample(0); s.Connected || s.DisplayOn {
		t.Fatalf("expected display off: connected=%v on=%v", s.Connected, s.DisplayOn)
	}
}

func TestManualModeIgnoresSessions(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Mode = config.ModeManual })
	h.settle()
	if s := h.sample(500); !s.Connected || s.DisplayOn {
		t.Fatalf("manual mode must not auto-create: %+v", s.DisplayOn)
	}
	h.ctl.CreateNow()
	if s := h.settle(); !s.DisplayOn {
		t.Fatal("CreateNow did not create")
	}
	h.ctl.RemoveNow()
	if s := h.settle(); s.DisplayOn {
		t.Fatal("RemoveNow did not remove")
	}
}

func TestRemoveDuringSessionSuppressesUntilItEnds(t *testing.T) {
	h := newHarness(t, nil)
	h.settle()
	h.sample(500)
	h.ctl.RemoveNow()
	if s := h.settle(); s.DisplayOn || !s.Suppressed {
		t.Fatalf("should stay removed: on=%v suppressed=%v", s.DisplayOn, s.Suppressed)
	}
	if s := h.sample(500); s.DisplayOn {
		t.Fatal("re-created during the same session")
	}
	for range 4 {
		h.sample(0)
	}
	if s := h.sample(500); !s.DisplayOn {
		t.Fatal("next session should create the display again")
	}
}

func TestConfigChangeRecreates(t *testing.T) {
	h := newHarness(t, nil)
	h.settle()
	h.sample(500)
	cfg := config.Default()
	cfg.Detect = detector.Params{ConnectKBps: 100, ConnectSeconds: 1, HoldKBps: 5, DisconnectSeconds: 3}
	cfg.Device.Portrait = true
	h.ctl.SetConfig(cfg)
	h.settle()
	if h.disp.creates != 2 || h.disp.last.Selected.Width != 1032 {
		t.Fatalf("expected portrait recreate, creates=%d mode=%+v", h.disp.creates, h.disp.last.Selected)
	}
}

func TestCreateFailureBacksOff(t *testing.T) {
	h := newHarness(t, nil)
	h.disp.fail = errors.New("boom")
	h.settle()
	s := h.sample(500)
	if s.DisplayOn || s.LastErr == "" {
		t.Fatalf("expected failure: %+v", s.LastErr)
	}
	h.sample(500)
	h.sample(500)
	if h.disp.creates != 1 {
		t.Fatalf("retried too soon: %d creates", h.disp.creates)
	}
}
