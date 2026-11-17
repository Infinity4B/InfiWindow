// Package vdisplay creates a virtual monitor sized like the remote device and
// arranges it so that remote-desktop apps stream it.
package vdisplay

import (
	"errors"
	"hash/fnv"

	"github.com/Infinity4B/infiwindow/internal/device"
)

// Layout says how the virtual display joins the existing ones.
type Layout string

const (
	// LayoutMirror makes every physical display mirror the virtual one, so
	// whichever screen the remote app captures shows the iPad-sized desktop.
	LayoutMirror Layout = "mirror"
	// LayoutMain makes the virtual display the main display (menu bar, Dock).
	LayoutMain Layout = "main"
	// LayoutExtend only adds the display beside the others.
	LayoutExtend Layout = "extend"
)

// ErrUnsupported is returned on systems without CGVirtualDisplay.
var ErrUnsupported = errors.New("当前系统不支持虚拟显示器（需要 macOS 11 及以上）")

// Display describes one online display.
type Display struct {
	ID          uint32  `json:"id"`
	X           int     `json:"x"`
	Y           int     `json:"y"`
	Width       int     `json:"width"`  // points
	Height      int     `json:"height"` // points
	PixelWidth  int     `json:"pixelWidth"`
	PixelHeight int     `json:"pixelHeight"`
	RefreshHz   float64 `json:"refreshHz"`
	Main        bool    `json:"main"`
	Builtin     bool    `json:"builtin"`
	Virtual     bool    `json:"virtual"`  // created by InfiWindow
	MirrorOf    uint32  `json:"mirrorOf"` // 0 when not mirroring
}

// Serial derives a stable serial number from the spec, so macOS remembers
// arrangement and colour settings per resolution/orientation.
func Serial(s device.Spec) uint32 {
	h := fnv.New32a()
	for _, m := range s.Modes {
		h.Write([]byte{byte(m.Width >> 8), byte(m.Width), byte(m.Height >> 8), byte(m.Height)})
	}
	v := h.Sum32()
	if v == 0 {
		v = 1
	}
	return v
}
