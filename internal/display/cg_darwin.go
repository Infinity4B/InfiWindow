//go:build darwin

package display

// The virtual display is built on CGVirtualDisplay, a private Objective-C API
// inside CoreGraphics (macOS 11+) that also powers DeskPad, BetterDisplay and
// Chromium's display tests. Everything is called through purego, so the app
// stays cgo-free as MyGo requires.

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

const (
	vendorID  = 0x1F4B // arbitrary but stable, so macOS remembers our settings
	productID = 0x1A1D

	configureForSession  = 1 // kCGConfigureForSession: reverted at logout
	configurePermanently = 2 // kCGConfigurePermanently: like System Settings
	nullDisplay          = 0
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
	cgModeIOID           func(mode uintptr) int32
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
	cgVendorNumber       func(display uint32) uint32
	cgModelNumber        func(display uint32) uint32
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
		purego.RegisterLibFunc(&cgModeIOID, cg, "CGDisplayModeGetIODisplayModeID")
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
		purego.RegisterLibFunc(&cgVendorNumber, cg, "CGDisplayVendorNumber")
		purego.RegisterLibFunc(&cgModelNumber, cg, "CGDisplayModelNumber")
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
		queue = dispatchQueueCreate(cstr("com.infinity4b.infiwindow.display"), 0)
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
