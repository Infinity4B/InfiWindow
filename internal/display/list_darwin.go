//go:build darwin

package display

import (
	"fmt"
	"math"
	"slices"

	"github.com/ebitengine/purego/objc"
)

func onlineDisplays() []uint32 {
	ids := make([]uint32, 16)
	var n uint32
	if cgOnlineList(uint32(len(ids)), &ids[0], &n) != 0 {
		return nil
	}
	return ids[:n]
}

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
			if v := cgVendorNumber(d); !info.Builtin && !info.Virtual && v != vendorID {
				info.VendorID, info.ProductID = v, cgModelNumber(d)
				info.NativeW, info.NativeH, info.Scaled = describeModes(d)
			}
			out = append(out, info)
		}
	})
	return out
}

// allModes lists every mode of a display, HiDPI ones included. The caller
// releases the array.
func allModes(d uint32) uintptr {
	yes := objc.ID(clsNSNumber).Send(sel("numberWithBool:"), true)
	opts := objc.ID(clsNSDictionary).Send(sel("dictionaryWithObject:forKey:"), yes, objc.ID(kShowDuplicateLowRes))
	return cgCopyAllModes(d, uintptr(opts))
}

// describeModes returns the largest 1x mode of a display and its HiDPI
// sizes, largest first.
func describeModes(d uint32) (nativeW, nativeH int, scaled []ScaledSize) {
	all := allModes(d)
	if all == 0 {
		return 0, 0, nil
	}
	defer cfRelease(all)
	seen := map[ScaledSize]bool{}
	for i := range cfArrayGetCount(all) {
		md := cfArrayGetValueAt(all, i)
		w, h := int(cgModeWidth(md)), int(cgModeHeight(md))
		pw, ph := int(cgModePixelWidth(md)), int(cgModePixelHeight(md))
		switch {
		case pw == w && pw*ph > nativeW*nativeH:
			nativeW, nativeH = pw, ph
		case pw == 2*w:
			if s := (ScaledSize{W: w, H: h}); !seen[s] {
				seen[s] = true
				scaled = append(scaled, s)
			}
		}
	}
	slices.SortFunc(scaled, func(a, b ScaledSize) int { return b.W*b.H - a.W*a.H })
	return nativeW, nativeH, scaled
}

// SetMode switches a display to w×h points, backed by 2x pixels when hiDPI,
// and keeps it like a choice in System Settings. It prefers the current
// refresh rate, else the highest: a monitor may list the same size at a low
// rate too when that mode is really a 4K signal.
func SetMode(id uint32, w, h int, hiDPI bool) error {
	if err := load(); err != nil {
		return err
	}
	var err error
	withPool(func() {
		cur := 0.0
		if md := cgCopyMode(id); md != 0 {
			cur = cgModeRefresh(md)
			cgModeRelease(md)
		}
		all := allModes(id)
		if all == 0 {
			err = fmt.Errorf("读取显示器 %d 的模式失败", id)
			return
		}
		defer cfRelease(all)
		pw := w
		if hiDPI {
			pw = 2 * w
		}
		var best uintptr
		bestHz := -1.0
		for i := range cfArrayGetCount(all) {
			md := cfArrayGetValueAt(all, i)
			if int(cgModeWidth(md)) != w || int(cgModeHeight(md)) != h || int(cgModePixelWidth(md)) != pw {
				continue
			}
			hz := cgModeRefresh(md)
			if best == 0 || math.Abs(hz-cur) < 0.5 || (math.Abs(bestHz-cur) >= 0.5 && hz > bestHz) {
				best, bestHz = md, hz
			}
		}
		if best == 0 {
			err = fmt.Errorf("显示器没有 %d×%d 的模式", w, h)
			return
		}
		var cfg uintptr
		if code := cgBeginConfig(&cfg); code != 0 {
			err = fmt.Errorf("切换分辨率失败 (CGError %d)", code)
			return
		}
		cgConfigureMode(cfg, id, best, 0)
		if code := cgCompleteConfig(cfg, configurePermanently); code != 0 {
			err = fmt.Errorf("切换分辨率失败 (CGError %d)", code)
		}
	})
	return err
}
