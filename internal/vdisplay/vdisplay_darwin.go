//go:build darwin

package vdisplay

// The virtual display is built on CGVirtualDisplay, a private Objective-C API
// inside CoreGraphics (macOS 11+) that also powers DeskPad, BetterDisplay and
// Chromium's display tests. Everything is called through purego, so the app
// stays cgo-free as MyGo requires.

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/Infinity4B/infiwindow/internal/device"
)

const (
	vendorID  = 0x1F4B // arbitrary but stable, so macOS remembers our settings
	productID = 0x1A1D

	configureForSession = 1 // kCGConfigureForSession: reverted at logout
	nullDisplay         = 0
)

type cgPoint struct{ X, Y float64 }
type cgSize struct{ W, H float64 }
type cgRect struct {
	Origin cgPoint
	Size   cgSize
}

var (
	loadOnce sync.Once
	loadErr  error

	// CoreFoundation / CoreGraphics
	cfArrayGetCount      func(arr uintptr) int
	cfArrayGetValueAt    func(arr uintptr, idx int) uintptr
	cfRelease            func(ref uintptr)
	cgCopyAllModes       func(display uint32, opts uintptr) uintptr
	cgCopyMode           func(display uint32) uintptr
	cgModeRelease        func(mode uintptr)
	cgModeWidth          func(mode uintptr) uint
	cgModeHeight         func(mode uintptr) uint
	cgModePixelWidth     func(mode uintptr) uint
	cgModePixelHeight    func(mode uintptr) uint
	cgModeRefresh        func(mode uintptr) float64
	cgBeginConfig        func(cfg *uintptr) int32
	cgConfigureMode      func(cfg uintptr, display uint32, mode uintptr, opts uintptr) int32
	cgConfigureMirror    func(cfg uintptr, display uint32, master uint32) int32
	cgConfigureOrigin    func(cfg uintptr, display uint32, x, y int32) int32
	cgCompleteConfig     func(cfg uintptr, option uint32) int32
	cgCancelConfig       func(cfg uintptr) int32
	cgOnlineList         func(max uint32, ids *uint32, count *uint32) int32
	cgMirrorsDisplay     func(display uint32) uint32
	cgMainDisplay        func() uint32
	cgIsOnline           func(display uint32) uint32
	cgIsMain             func(display uint32) uint32
	cgIsBuiltin          func(display uint32) uint32
	cgBounds             func(display uint32) cgRect
	dispatchQueueCreate  func(label *byte, attr uintptr) uintptr
	autoreleasePoolPush  func() uintptr
	autoreleasePoolPop   func(pool uintptr)
	kShowDuplicateLowRes uintptr // CFStringRef
	clsDescriptor        objc.Class
	clsDisplay           objc.Class
	clsSettings          objc.Class
	clsMode              objc.Class
	clsNSString          objc.Class
	clsNSMutableArray    objc.Class
	clsNSDictionary      objc.Class
	clsNSNumber          objc.Class
	selAlloc             = objc.RegisterName("alloc")
	selInit              = objc.RegisterName("init")
	selRelease           = objc.RegisterName("release")
	queue                uintptr
)

func load() error {
	loadOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				loadErr = fmt.Errorf("加载系统框架失败: %v", r)
			}
		}()
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = err
			return
		}
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = err
			return
		}
		if _, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			loadErr = err
			return
		}
		sys, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = err
			return
		}
		objcLib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = err
			return
		}

		purego.RegisterLibFunc(&cfArrayGetCount, cf, "CFArrayGetCount")
		purego.RegisterLibFunc(&cfArrayGetValueAt, cf, "CFArrayGetValueAtIndex")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&cgCopyAllModes, cg, "CGDisplayCopyAllDisplayModes")
		purego.RegisterLibFunc(&cgCopyMode, cg, "CGDisplayCopyDisplayMode")
		purego.RegisterLibFunc(&cgModeRelease, cg, "CGDisplayModeRelease")
		purego.RegisterLibFunc(&cgModeWidth, cg, "CGDisplayModeGetWidth")
		purego.RegisterLibFunc(&cgModeHeight, cg, "CGDisplayModeGetHeight")
		purego.RegisterLibFunc(&cgModePixelWidth, cg, "CGDisplayModeGetPixelWidth")
		purego.RegisterLibFunc(&cgModePixelHeight, cg, "CGDisplayModeGetPixelHeight")
		purego.RegisterLibFunc(&cgModeRefresh, cg, "CGDisplayModeGetRefreshRate")
		purego.RegisterLibFunc(&cgBeginConfig, cg, "CGBeginDisplayConfiguration")
		purego.RegisterLibFunc(&cgConfigureMode, cg, "CGConfigureDisplayWithDisplayMode")
		purego.RegisterLibFunc(&cgConfigureMirror, cg, "CGConfigureDisplayMirrorOfDisplay")
		purego.RegisterLibFunc(&cgConfigureOrigin, cg, "CGConfigureDisplayOrigin")
		purego.RegisterLibFunc(&cgCompleteConfig, cg, "CGCompleteDisplayConfiguration")
		purego.RegisterLibFunc(&cgCancelConfig, cg, "CGCancelDisplayConfiguration")
		purego.RegisterLibFunc(&cgOnlineList, cg, "CGGetOnlineDisplayList")
		purego.RegisterLibFunc(&cgMirrorsDisplay, cg, "CGDisplayMirrorsDisplay")
		purego.RegisterLibFunc(&cgMainDisplay, cg, "CGMainDisplayID")
		purego.RegisterLibFunc(&cgIsOnline, cg, "CGDisplayIsOnline")
		purego.RegisterLibFunc(&cgIsMain, cg, "CGDisplayIsMain")
		purego.RegisterLibFunc(&cgIsBuiltin, cg, "CGDisplayIsBuiltin")
		purego.RegisterLibFunc(&cgBounds, cg, "CGDisplayBounds")
		purego.RegisterLibFunc(&dispatchQueueCreate, sys, "dispatch_queue_create")
		purego.RegisterLibFunc(&autoreleasePoolPush, objcLib, "objc_autoreleasePoolPush")
		purego.RegisterLibFunc(&autoreleasePoolPop, objcLib, "objc_autoreleasePoolPop")

		sym, err := purego.Dlsym(cg, "kCGDisplayShowDuplicateLowResolutionModes")
		if err != nil {
			loadErr = err
			return
		}
		// sym is the address of a CFStringRef global; read it without vet's uintptr→Pointer warning.
		kShowDuplicateLowRes = **(**uintptr)(unsafe.Pointer(&sym))

		clsDescriptor = objc.GetClass("CGVirtualDisplayDescriptor")
		clsDisplay = objc.GetClass("CGVirtualDisplay")
		clsSettings = objc.GetClass("CGVirtualDisplaySettings")
		clsMode = objc.GetClass("CGVirtualDisplayMode")
		clsNSString = objc.GetClass("NSString")
		clsNSMutableArray = objc.GetClass("NSMutableArray")
		clsNSDictionary = objc.GetClass("NSDictionary")
		clsNSNumber = objc.GetClass("NSNumber")
		if clsDescriptor == 0 || clsDisplay == 0 || clsSettings == 0 || clsMode == 0 {
			loadErr = ErrUnsupported
			return
		}
		queue = dispatchQueueCreate(cstr("com.infinity4b.infiwindow.vdisplay"), 0)
	})
	return loadErr
}

func cstr(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}

func sel(name string) objc.SEL { return objc.RegisterName(name) }

func newObj(c objc.Class) objc.ID {
	return objc.ID(c).Send(selAlloc).Send(selInit)
}

// withPool runs fn on one locked OS thread inside an autorelease pool:
// Go threads have none, and CoreGraphics autoreleases objects.
func withPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := autoreleasePoolPush()
	defer autoreleasePoolPop(pool)
	fn()
}

// Manager owns at most one virtual display.
type Manager struct {
	mu      sync.Mutex
	display objc.ID
	id      uint32
	layout  Layout
	// prevMain is the main display before LayoutMain moved it.
	prevMain uint32
}

func NewManager() *Manager { return &Manager{} }

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
func (m *Manager) Create(spec device.Spec, layout Layout) (uint32, error) {
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
	withPool(func() { id, err = m.createLocked(spec) })
	if err != nil {
		return 0, err
	}

	if !selectMode(id, spec.Selected, spec.RefreshHz, 3*time.Second) {
		m.destroyLocked(false)
		return 0, fmt.Errorf("系统未提供 %d×%d 的显示模式", spec.Selected.Width, spec.Selected.Height)
	}
	m.layout = layout
	if code := m.applyLayout(layout); code != 0 {
		// The display works; only the arrangement failed.
		return id, fmt.Errorf("虚拟显示器已创建，但排列失败 (CGError %d)", code)
	}
	return id, nil
}

func (m *Manager) createLocked(spec device.Spec) (uint32, error) {
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
func selectMode(id uint32, want device.Mode, hz float64, timeout time.Duration) bool {
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

func trySelectMode(id uint32, want device.Mode, hz float64) bool {
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

func onlineDisplays() []uint32 {
	ids := make([]uint32, 16)
	var n uint32
	if cgOnlineList(uint32(len(ids)), &ids[0], &n) != 0 {
		return nil
	}
	return ids[:n]
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
	withPool(func() {
		if m.restoreLayout() && wait {
			m.waitSettled(1500 * time.Millisecond)
		}
		// Releasing the CGVirtualDisplay removes the monitor right away.
		m.display.Send(selRelease)
	})
	m.display, m.id = 0, 0
}

// ListDisplays returns every online display.
func ListDisplays() []Display {
	return listDisplays(0)
}

func (m *Manager) ListDisplays() []Display {
	m.mu.Lock()
	id := m.id
	m.mu.Unlock()
	return listDisplays(id)
}

func listDisplays(ours uint32) []Display {
	if load() != nil {
		return nil
	}
	var out []Display
	withPool(func() {
		for _, d := range onlineDisplays() {
			b := cgBounds(d)
			info := Display{
				ID: d, X: int(b.Origin.X), Y: int(b.Origin.Y),
				Width: int(b.Size.W), Height: int(b.Size.H),
				Main: cgIsMain(d) != 0, Builtin: cgIsBuiltin(d) != 0,
				Virtual: ours != 0 && d == ours, MirrorOf: cgMirrorsDisplay(d),
			}
			if md := cgCopyMode(d); md != 0 {
				info.PixelWidth = int(cgModePixelWidth(md))
				info.PixelHeight = int(cgModePixelHeight(md))
				info.RefreshHz = cgModeRefresh(md)
				cgModeRelease(md)
			}
			out = append(out, info)
		}
	})
	return out
}
