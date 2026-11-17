// InfiWindow adds a virtual monitor sized for an iPad (iPad Pro 13" M4 by
// default) whenever a UU Remote (UU 远程) session streams this Mac, so the
// remote desktop fills the iPad screen instead of being letterboxed.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infiwindow/internal/config"
	"github.com/Infinity4B/infiwindow/internal/controller"
	"github.com/Infinity4B/infiwindow/internal/detector"
	"github.com/Infinity4B/infiwindow/internal/vdisplay"
)

func main() {
	mygo.App.SetName("InfiWindow")
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running instance shows its window
	}
	// A menu bar utility: no Dock icon, the window opens from the tray.
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)

	a := &app{}
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

func (a *app) ready() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Printf("user data dir: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("config: %v (using defaults)", err)
	}
	a.edit = cfg
	a.openAtLogin = mygo.App.OpenAtLogin()
	a.syncPresetName()

	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:         "InfiWindow",
		Width:         980,
		Height:        720,
		MinWidth:      820,
		MinHeight:     560,
		StateKey:      "main",
		TitleBarStyle: mygo.TitleBarHidden,
		Hidden:        mygo.App.WasOpenedAtLogin(),
		Content:       ui.View(a.view),
	})
	a.win.OnClose(func(e *mygo.CloseEvent) {
		// Closing only hides: the app keeps watching from the menu bar.
		if !a.quitting {
			e.PreventDefault()
			a.win.Hide()
		}
	})

	a.setupTray()

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.ctl = controller.New(cfg, vdisplay.NewManager(), detector.NewWatcher(cfg.Processes),
		func(c config.Config) error { return config.Save(cfgPath, c) },
		func(s controller.Snapshot) {
			a.win.Update(func() {
				a.snap = s
				a.hasSnap = true
				a.updateTray()
			})
		})
	go a.ctl.Run(ctx)
	a.handleArgs(os.Args[1:])
}

// handleArgs runs command-line actions, so scripts and shortcuts can drive a
// running InfiWindow:
//
//	open -a InfiWindow --args --create   # turn the virtual display on
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
