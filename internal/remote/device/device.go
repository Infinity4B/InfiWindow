// Package device describes iPad models and turns user preferences into a
// concrete virtual display specification.
package device

import (
	"fmt"
	"math"

	"github.com/Infinity4B/infiwindow/internal/display"
)

// Preset is a physical device the virtual display should imitate.
// Pixel sizes are given in landscape orientation (width > height).
type Preset struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	PixelW int     `json:"pixelW"`
	PixelH int     `json:"pixelH"`
	PPI    float64 `json:"ppi"`
	MaxHz  int     `json:"maxHz"`
}

// DefaultPresetID is the iPad Pro 13" (M4), the device InfiWindow targets by default.
const DefaultPresetID = "ipad-pro-13-m4"

// Presets lists the supported devices. The first entry is the default.
var Presets = []Preset{
	{ID: "ipad-pro-13-m4", Name: `iPad Pro 13" (M4)`, PixelW: 2752, PixelH: 2064, PPI: 264, MaxHz: 120},
	{ID: "ipad-pro-11-m4", Name: `iPad Pro 11" (M4)`, PixelW: 2420, PixelH: 1668, PPI: 264, MaxHz: 120},
	{ID: "ipad-pro-12.9", Name: `iPad Pro 12.9" (第 3-6 代)`, PixelW: 2732, PixelH: 2048, PPI: 264, MaxHz: 120},
	{ID: "ipad-air-13", Name: `iPad Air 13" (M2/M3)`, PixelW: 2732, PixelH: 2048, PPI: 264, MaxHz: 60},
	{ID: "ipad-air-11", Name: `iPad Air 11" (M2/M3)`, PixelW: 2360, PixelH: 1640, PPI: 264, MaxHz: 60},
	{ID: "ipad-mini-a17", Name: "iPad mini (A17 Pro)", PixelW: 2266, PixelH: 1488, PPI: 326, MaxHz: 60},
}

// Find returns the preset with the given id, or the default preset.
func Find(id string) Preset {
	for _, p := range Presets {
		if p.ID == id {
			return p
		}
	}
	return Presets[0]
}

// Scale options. HiDPI scales render at 2x backing resolution.
const (
	ScaleLarger  = "larger"  // bigger UI, less space
	ScaleDefault = "default" // the iPad's own Retina point size (pixels / 2)
	ScaleMore    = "more"    // more space, smaller UI
	ScaleNative  = "native"  // 1:1 pixels, no HiDPI (tiny UI)
)

var scaleFactors = map[string]float64{
	ScaleLarger:  0.875,
	ScaleDefault: 1.0,
	ScaleMore:    1.15,
}

// Mode and Spec describe the virtual display; they live in package display,
// which creates it.
type (
	Mode = display.Mode
	Spec = display.Spec
)

// Options are the user-facing knobs.
type Options struct {
	PresetID  string `json:"presetId"`
	Portrait  bool   `json:"portrait"`
	Scale     string `json:"scale"`
	RefreshHz int    `json:"refreshHz"`
	// Custom overrides the preset when both are > 0 (landscape pixels).
	CustomW int `json:"customW"`
	CustomH int `json:"customH"`
}

func even(v float64) int {
	n := int(math.Round(v))
	if n%2 != 0 {
		n++
	}
	return n
}

// BuildSpec converts options into a display spec.
func BuildSpec(o Options) (Spec, error) {
	p := Find(o.PresetID)
	pw, ph := p.PixelW, p.PixelH
	if o.CustomW > 0 && o.CustomH > 0 {
		if o.CustomW < 640 || o.CustomH < 480 || o.CustomW > 7680 || o.CustomH > 4320 {
			return Spec{}, fmt.Errorf("自定义分辨率超出范围 (640×480 ~ 7680×4320)")
		}
		pw, ph = o.CustomW, o.CustomH
		if ph > pw {
			pw, ph = ph, pw
		}
	}
	if o.Portrait {
		pw, ph = ph, pw
	}

	hz := float64(o.RefreshHz)
	if hz != 60 && hz != 120 {
		hz = 60
	}

	ppi := p.PPI
	if ppi <= 0 {
		ppi = 264
	}

	spec := Spec{
		Name:      "InfiWindow · " + p.Name,
		RefreshHz: hz,
		WidthMM:   math.Round(float64(pw)/ppi*25.4*10) / 10,
		HeightMM:  math.Round(float64(ph)/ppi*25.4*10) / 10,
	}
	if o.CustomW > 0 && o.CustomH > 0 {
		spec.Name = fmt.Sprintf("InfiWindow · %d×%d", pw, ph)
	}

	baseW, baseH := float64(pw)/2, float64(ph)/2
	// Register every HiDPI scale plus the native 1:1 mode so the user can
	// also switch from System Settings without recreating the display.
	for _, key := range []string{ScaleLarger, ScaleDefault, ScaleMore} {
		f := scaleFactors[key]
		m := Mode{Width: even(baseW * f), Height: even(baseH * f), HiDPI: true}
		spec.Modes = append(spec.Modes, m)
		if key == o.Scale {
			spec.Selected = m
		}
	}
	native := Mode{Width: pw, Height: ph, HiDPI: false}
	spec.Modes = append(spec.Modes, native)
	if o.Scale == ScaleNative {
		spec.Selected = native
	}
	if spec.Selected.Width == 0 {
		spec.Selected = spec.Modes[1] // default scale
	}

	for _, m := range spec.Modes {
		w, h := m.PixelSize()
		spec.MaxPixelW = max(spec.MaxPixelW, w)
		spec.MaxPixelH = max(spec.MaxPixelH, h)
	}
	return spec, nil
}
