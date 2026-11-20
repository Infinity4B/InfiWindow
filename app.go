package main

import (
	"context"

	"github.com/egoist/mygo"

	"github.com/Infinity4B/infiwindow/internal/activity"
	"github.com/Infinity4B/infiwindow/internal/display"
	"github.com/Infinity4B/infiwindow/internal/remote"
)

// Sources of activity log entries besides remote.LogSource.
const (
	appSource   = "InfiWindow"
	hidpiSource = "外接屏 HiDPI"
)

// app is the UI state. Everything here is read and written on the main
// thread; goroutines hand over their results with win.Update.
type app struct {
	win    *mygo.Window
	tray   *mygo.Tray
	cancel context.CancelFunc
	log    *activity.Log

	trayRemote, trayToggle, trayAuto, trayHiDPI *mygo.MenuItem
	trayOn, trayInit                            bool

	quitting    bool
	page        string
	openAtLogin bool

	// displays is every online display, refreshed by watchDisplays.
	displays []display.Display
	kick     chan struct{}

	// The remote display feature: its controller, latest snapshot, and the
	// settings the page edits (every change goes to the controller, which
	// saves it).
	ctl        *remote.Controller
	snap       remote.Snapshot
	hasSnap    bool
	edit       remote.Config
	presetName string
	advanced   bool
	procText   string
	procInit   bool

	hidpi hidpiState
}

func newApp() *app {
	a := &app{page: pageOverview, kick: make(chan struct{}, 1)}
	a.log = activity.New(func() {
		if a.win != nil {
			a.win.Invalidate()
		}
	})
	return a
}
