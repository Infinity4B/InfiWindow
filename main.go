// InfiWindow is a menu bar toolkit for the Mac's displays. Each feature is
// independent:
//
//   - Remote display: while UU Remote (UU 远程) streams this Mac to an iPad,
//     a virtual monitor sized like the iPad is added, so the remote desktop
//     fills its screen instead of being letterboxed.
//   - External-monitor HiDPI: scaled HiDPI resolutions for non-Retina
//     monitors such as 2K panels, through a display override file.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/activity"
	"github.com/Infinity4B/infiwindow/internal/config"
	"github.com/Infinity4B/infiwindow/internal/display"
	"github.com/Infinity4B/infiwindow/internal/remote"
	"github.com/Infinity4B/infiwindow/internal/remote/detector"
)

func main() {
	mygo.App.SetName("InfiWindow")
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running instance shows its window
	}
	// A menu bar utility: no Dock icon, the window opens from the tray.
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)

	a := newApp()
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if !a.handleArgs(args) {
			a.showWindow()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnQuitCanceled(func() { a.quitting = false })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.shutdown() })
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// store keeps the whole configuration so each feature can save its own
// section from its own goroutine.
type store struct {
	mu   sync.Mutex
	path string
	cfg  config.Config
}

func (s *store) saveRemote(r remote.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Remote = r
	return config.Save(s.path, s.cfg)
}

func (a *app) ready() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Printf("user data dir: %v", err)
	}
	st := &store{path: filepath.Join(dir, "config.json")}
	st.cfg, err = config.Load(st.path)
	if err != nil {
		log.Printf("config: %v (using defaults)", err)
	}
	a.edit = st.cfg.Remote
	a.openAtLogin = mygo.App.OpenAtLogin()
	a.syncPresetName()

	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:         "InfiWindow",
		Width:         1080,
		Height:        740,
		MinWidth:      900,
		MinHeight:     560,
		StateKey:      "main",
		TitleBarStyle: mygo.TitleBarHidden,
		Hidden:        mygo.App.WasOpenedAtLogin(),
		Content:       ui.View(a.view),
	})
	a.win.OnClose(func(e *mygo.CloseEvent) {
		// Closing only hides: the app keeps working from the menu bar.
		if !a.quitting {
			e.PreventDefault()
			a.win.Hide()
		}
	})
	a.log.Add(appSource, activity.Info, "InfiWindow 已启动")

	a.setupTray()

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	manager := display.NewManager()
	a.ctl = remote.New(st.cfg.Remote, manager, detector.NewWatcher(st.cfg.Remote.Processes), a.log, st.saveRemote,
		func(s remote.Snapshot) {
			a.win.Update(func() {
				prev := a.snap
				a.snap = s
				a.hasSnap = true
				if prev.DisplayOn != s.DisplayOn || prev.DisplayID != s.DisplayID || prev.Busy != s.Busy {
					a.refreshDisplays()
				}
				a.updateTray()
			})
		})
	go a.ctl.Run(ctx)
	go a.watchDisplays(ctx, manager.ListDisplays)
	a.handleArgs(os.Args[1:])
}

// watchDisplays lists the displays every few seconds, and right away when
// refreshDisplays asks, for every page that shows them.
func (a *app) watchDisplays(ctx context.Context, list func() []display.Display) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		ds := list()
		a.win.Update(func() {
			a.displays = ds
			a.refreshOverrides()
		})
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-a.kick:
		}
	}
}

// refreshDisplays asks watchDisplays for a new list. Any goroutine.
func (a *app) refreshDisplays() {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

// handleArgs runs command-line actions, so scripts and shortcuts can drive a
// running InfiWindow:
//
//	open -a InfiWindow --args --create   # turn the remote display on
//	open -a InfiWindow --args --remove   # turn it off
//
// It reports whether an action was found.
func (a *app) handleArgs(args []string) bool {
	found := false
	for _, arg := range args {
		switch arg {
		case "--create":
			a.ctl.CreateNow()
			found = true
		case "--remove":
			a.ctl.RemoveNow()
			found = true
		case "--quit":
			mygo.App.Quit()
			found = true
		}
	}
	return found
}

// shutdown removes the virtual display before the process exits.
func (a *app) shutdown() {
	if a.cancel == nil {
		return
	}
	a.cancel()
	select {
	case <-a.ctl.Done():
	case <-time.After(3 * time.Second):
	}
	if a.tray != nil {
		a.tray.Destroy()
	}
}

func (a *app) showWindow() {
	if a.win == nil {
		return
	}
	a.win.Show()
	a.win.Restore()
	a.win.Focus()
	mygo.App.Focus()
}
