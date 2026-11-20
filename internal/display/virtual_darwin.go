//go:build darwin

package display

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/purego/objc"
)

// Manager owns at most one virtual display.
type Manager struct {
	mu      sync.Mutex
	display objc.ID
	id      uint32
	layout  Layout
	// prevMain is the main display before LayoutMain moved it.
	prevMain uint32
	// base learns the mode the user chose for each display, see TrackModes.
	base *baseline[modeKey]
	// saved are the modes to put back once the virtual display is gone:
	// mirroring switches the other displays to a mode that fits the iPad,
	// and when it ends macOS picks one by itself, often another resolution
	// or a fixed 47.95 Hz instead of ProMotion.
	saved []savedMode
}

// savedMode is a display mode remembered for restoring.
type savedMode struct {
	display uint32
	key     modeKey
}

type modeKey struct {
	ioID           int32
	w, h, pw, ph   int
	refreshMilliHz int
}

func keyOf(md uintptr) modeKey {
	return modeKey{
		ioID: cgModeIOID(md),
		w:    int(cgModeWidth(md)), h: int(cgModeHeight(md)),
		pw: int(cgModePixelWidth(md)), ph: int(cgModePixelHeight(md)),
		refreshMilliHz: int(math.Round(cgModeRefresh(md) * 1000)),
	}
}

// sameSize reports whether a and b differ at most in the mode ID, which
// macOS may renumber when it rebuilds a display's mode list.
func (k modeKey) sameSize(o modeKey) bool {
	k.ioID, o.ioID = 0, 0
	return k == o
}

func NewManager() *Manager { return &Manager{base: newBaseline[modeKey](ModeSettle)} }

// TrackModes learns which mode the user chose for each display. Call it
// regularly; idle says no remote session is running or starting. Remote
// desktop apps change the resolution as they connect, before the session is
// detected, so the modes seen when the virtual display is created are not
// necessarily the user's. Only a mode that stays for ModeSettle while idle
// and without a virtual display is learned.
func (m *Manager) TrackModes(idle bool) {
	if load() != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !idle || m.display != 0 {
		m.base.interrupt()
		return
	}
	m.base.observe(time.Now(), currentModes())
}

// AdoptModes takes the current modes as the user's choice right away, for
// when the user turns the display on by hand outside a session and may just
// have picked a resolution.
func (m *Manager) AdoptModes() {
	if load() != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.display != 0 {
		return
	}
	m.base.adopt(currentModes())
}

// currentModes returns the mode of every online display.
func currentModes() map[uint32]modeKey {
	out := map[uint32]modeKey{}
	withPool(func() {
		for _, d := range onlineDisplays() {
			if md := cgCopyMode(d); md != 0 {
				out[d] = keyOf(md)
				cgModeRelease(md)
			}
		}
	})
	return out
}

// Supported reports whether CGVirtualDisplay is available.
func (m *Manager) Supported() bool { return load() == nil }

// Active returns the current display id.
func (m *Manager) Active() (uint32, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.display == 0 || load() != nil || cgIsOnline(m.id) == 0 {
		return m.id, false
	}
	return m.id, true
}

// Create replaces any existing virtual display with one built from spec and
// arranges it according to layout.
func (m *Manager) Create(spec Spec, layout Layout) (uint32, error) {
	if err := load(); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyLocked(true)

	var (
		id  uint32
		err error
	)
	withPool(func() {
		m.saveModes()
		id, err = m.createLocked(spec)
		if err != nil {
			m.saved = nil
		}
	})
	if err != nil {
		return 0, err
	}

	if !selectMode(id, spec.Selected, spec.RefreshHz, 3*time.Second) {
		m.destroyLocked(true)
		return 0, fmt.Errorf("系统未提供 %d×%d 的显示模式", spec.Selected.Width, spec.Selected.Height)
	}
	m.layout = layout
	if code := m.applyLayout(layout); code != 0 {
		// The display works; only the arrangement failed.
		return id, fmt.Errorf("虚拟显示器已创建，但排列失败 (CGError %d)", code)
	}
	return id, nil
}

func (m *Manager) createLocked(spec Spec) (uint32, error) {
	desc := newObj(clsDescriptor)
	defer desc.Send(selRelease)
	name := objc.ID(clsNSString).Send(selAlloc).Send(sel("initWithUTF8String:"), cstr(spec.Name))
	defer name.Send(selRelease)

	desc.Send(sel("setQueue:"), queue)
	desc.Send(sel("setName:"), name)
	desc.Send(sel("setMaxPixelsWide:"), uint32(spec.MaxPixelW))
	desc.Send(sel("setMaxPixelsHigh:"), uint32(spec.MaxPixelH))
	desc.Send(sel("setSizeInMillimeters:"), cgSize{W: spec.WidthMM, H: spec.HeightMM})
	desc.Send(sel("setVendorID:"), uint32(vendorID))
	desc.Send(sel("setProductID:"), uint32(productID))
	desc.Send(sel("setSerialNum:"), Serial(spec))

	display := objc.ID(clsDisplay).Send(selAlloc).Send(sel("initWithDescriptor:"), desc)
	if display == 0 {
		return 0, fmt.Errorf("创建虚拟显示器失败")
	}
	id := objc.Send[uint32](display, sel("displayID"))
	if id == 0 {
		display.Send(selRelease)
		return 0, fmt.Errorf("创建虚拟显示器失败（没有显示器 ID）")
	}

	modes := objc.ID(clsNSMutableArray).Send(selAlloc).Send(sel("init"))
	defer modes.Send(selRelease)
	for _, md := range spec.Modes {
		mode := objc.ID(clsMode).Send(selAlloc).Send(sel("initWithWidth:height:refreshRate:"),
			uint32(md.Width), uint32(md.Height), spec.RefreshHz)
		modes.Send(sel("addObject:"), mode)
		mode.Send(selRelease)
	}
	settings := newObj(clsSettings)
	defer settings.Send(selRelease)
	// With hiDPI on, macOS offers each mode both at 1x and backed by 2x
	// pixels as long as the doubled size fits within maxPixels.
	settings.Send(sel("setHiDPI:"), uint32(1))
	settings.Send(sel("setModes:"), modes)
	if !objc.Send[bool](display, sel("applySettings:"), settings) {
		display.Send(selRelease)
		return 0, fmt.Errorf("虚拟显示器拒绝了分辨率设置")
	}
	m.display, m.id = display, id
	return id, nil
}

// selectMode waits for macOS to publish the requested mode, then switches to it.
func selectMode(id uint32, want Mode, hz float64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		var ok bool
		withPool(func() { ok = trySelectMode(id, want, hz) })
		if ok {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func trySelectMode(id uint32, want Mode, hz float64) bool {
	yes := objc.ID(clsNSNumber).Send(sel("numberWithBool:"), true)
	opts := objc.ID(clsNSDictionary).Send(sel("dictionaryWithObject:forKey:"), yes, objc.ID(kShowDuplicateLowRes))
	all := cgCopyAllModes(id, uintptr(opts))
	if all == 0 {
		return false
	}
	defer cfRelease(all)
	wantPW, _ := want.PixelSize()
	var best uintptr
	bestDelta := math.MaxFloat64
	for i := range cfArrayGetCount(all) {
		md := cfArrayGetValueAt(all, i)
		if int(cgModeWidth(md)) != want.Width || int(cgModeHeight(md)) != want.Height || int(cgModePixelWidth(md)) != wantPW {
			continue
		}
		if d := math.Abs(cgModeRefresh(md) - hz); d < bestDelta {
			best, bestDelta = md, d
		}
	}
	if best == 0 {
		return false
	}
	var cfg uintptr
	if cgBeginConfig(&cfg) != 0 {
		return false
	}
	cgConfigureMode(cfg, id, best, 0)
	return cgCompleteConfig(cfg, configureForSession) == 0
}

func (m *Manager) applyLayout(layout Layout) int32 {
	if layout == LayoutExtend || layout == "" {
		return 0
	}
	others := []uint32{}
	for _, d := range onlineDisplays() {
		if d != m.id {
			others = append(others, d)
		}
	}
	if len(others) == 0 {
		return 0 // headless Mac: the virtual display is already the only one
	}
	var cfg uintptr
	if code := cgBeginConfig(&cfg); code != 0 {
		return code
	}
	switch layout {
	case LayoutMirror:
		for _, d := range others {
			if cgMirrorsDisplay(d) == nullDisplay {
				cgConfigureMirror(cfg, d, m.id)
			}
		}
	case LayoutMain:
		// The main display sits at (0,0). Park the virtual display left of
		// the whole arrangement so nothing overlaps, then shift everything
		// so it lands on the origin.
		m.prevMain = cgMainDisplay()
		vb := cgBounds(m.id)
		minX := 0.0
		for _, d := range others {
			minX = math.Min(minX, cgBounds(d).Origin.X)
		}
		dx := vb.Size.W - minX
		cgConfigureOrigin(cfg, m.id, 0, 0)
		for _, d := range others {
			b := cgBounds(d)
			cgConfigureOrigin(cfg, d, int32(b.Origin.X+dx), int32(b.Origin.Y))
		}
	}
	return cgCompleteConfig(cfg, configureForSession)
}

// restoreLayout undoes mirroring and gives the main display back.
func (m *Manager) restoreLayout() bool {
	if m.id == 0 {
		return false
	}
	var cfg uintptr
	if cgBeginConfig(&cfg) != 0 {
		return false
	}
	changed := false
	ids := onlineDisplays()
	for _, d := range ids {
		if d != m.id && cgMirrorsDisplay(d) == m.id {
			cgConfigureMirror(cfg, d, nullDisplay)
			changed = true
		}
	}
	if m.prevMain != 0 && cgIsOnline(m.prevMain) != 0 && cgMainDisplay() == m.id {
		pb := cgBounds(m.prevMain)
		for _, d := range ids {
			if d == m.id {
				continue
			}
			b := cgBounds(d)
			cgConfigureOrigin(cfg, d, int32(b.Origin.X-pb.Origin.X), int32(b.Origin.Y-pb.Origin.Y))
		}
		changed = true
	}
	m.prevMain = 0
	if changed {
		cgCompleteConfig(cfg, configureForSession)
	} else {
		cgCancelConfig(cfg)
	}
	return changed
}

// waitSettled waits for WindowServer to finish undoing the layout: a virtual
// display released while it is still mirrored or main is not removed.
func (m *Manager) waitSettled(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		busy := cgMainDisplay() == m.id
		for _, d := range onlineDisplays() {
			if d != m.id && cgMirrorsDisplay(d) == m.id {
				busy = true
			}
		}
		if !busy {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
}

// Destroy restores the arrangement and removes the virtual display.
func (m *Manager) Destroy() {
	if load() != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyLocked(true)
}

// Close is Destroy for quitting: it does not wait for WindowServer, whose
// notifications need the main thread that is busy quitting. The process
// exiting removes the display regardless.
func (m *Manager) Close() {
	if load() != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyLocked(false)
}

func (m *Manager) destroyLocked(wait bool) {
	if m.display == 0 {
		return
	}
	id := m.id
	var relaid bool
	withPool(func() {
		relaid = m.restoreLayout()
		if relaid && wait {
			m.waitSettled(1500 * time.Millisecond)
		}
		// Releasing the CGVirtualDisplay removes the monitor right away.
		m.display.Send(selRelease)
	})
	m.display, m.id = 0, 0
	m.restoreModes(id, relaid, wait)
}

// saveModes picks the mode to restore for every online display: the one
// the user chose as learned by TrackModes, else the current one.
func (m *Manager) saveModes() {
	m.saved = nil
	for _, d := range onlineDisplays() {
		key, ok := m.base.get(d)
		if !ok {
			md := cgCopyMode(d)
			if md == 0 {
				continue
			}
			key = keyOf(md)
			cgModeRelease(md)
		}
		m.saved = append(m.saved, savedMode{display: d, key: key})
	}
}

// restoreModes puts the saved modes back once the virtual display vid is
// gone. macOS reconfigures the displays itself when the mirror set breaks up
// and when the virtual display disappears, so with wait it checks again
// after a moment and reapplies whatever was changed back. relaid says the
// mirroring or main display was undone: then the first pass sets every saved
// mode without comparing, since the current mode this process sees may not
// have caught up yet.
func (m *Manager) restoreModes(vid uint32, relaid, wait bool) {
	if len(m.saved) == 0 {
		return
	}
	defer func() { m.saved = nil }()
	if !wait {
		withPool(func() { m.applySavedModes(relaid) })
		return
	}
	deadline := time.Now().Add(2 * time.Second)
	for vid != 0 && cgIsOnline(vid) != 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	for attempt := 0; attempt < 3; attempt++ {
		time.Sleep(300 * time.Millisecond)
		var pending bool
		force := relaid && attempt == 0
		withPool(func() { pending = m.applySavedModes(force) })
		if !pending {
			return
		}
	}
}

// applySavedModes switches every display not in its saved mode back to it
// (every display when force) and reports whether any needed switching.
func (m *Manager) applySavedModes(force bool) bool {
	var cfg uintptr
	if cgBeginConfig(&cfg) != 0 {
		return true
	}
	changed := false
	for _, s := range m.saved {
		if cgIsOnline(s.display) == 0 {
			continue
		}
		cur := cgCopyMode(s.display)
		if cur == 0 {
			continue
		}
		same := keyOf(cur).sameSize(s.key)
		cgModeRelease(cur)
		if same && !force {
			continue
		}
		yes := objc.ID(clsNSNumber).Send(sel("numberWithBool:"), true)
		opts := objc.ID(clsNSDictionary).Send(sel("dictionaryWithObject:forKey:"), yes, objc.ID(kShowDuplicateLowRes))
		all := cgCopyAllModes(s.display, uintptr(opts))
		if all == 0 {
			continue
		}
		// The configuration uses the mode objects until it completes.
		defer cfRelease(all)
		var md uintptr
		for i := range cfArrayGetCount(all) {
			c := cfArrayGetValueAt(all, i)
			if k := keyOf(c); k == s.key {
				md = c
				break
			} else if md == 0 && k.sameSize(s.key) {
				md = c
			}
		}
		if md != 0 && cgConfigureMode(cfg, s.display, md, 0) == 0 {
			changed = true
		}
	}
	if !changed {
		cgCancelConfig(cfg)
		return false
	}
	cgCompleteConfig(cfg, configureForSession)
	return true
}

// ListDisplays returns every online display.
