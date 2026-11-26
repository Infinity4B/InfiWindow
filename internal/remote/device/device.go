// Package device describes remote client devices (iPad models and desktop
// monitors) and turns user preferences into a concrete virtual display spec.
package device

import (
	"fmt"
	"math"

	"github.com/Infinity4B/infiwindow/internal/display"
)

// Kind distinguishes client device families with different DPI and scaling
// expectations.
type Kind string

const (
	KindIPad Kind = "ipad"
	KindPC   Kind = "pc"
)

// KindLabel returns the Chinese UI label for a client device family.
func KindLabel(k Kind) string {
	if k == KindPC {
		return "电脑"
	}
	return "iPad"
}

// Preset is a physical device or desktop monitor the virtual display should
// imitate. Pixel sizes are given in landscape orientation (width > height).
type Preset struct {
	ID           string  `json:"id"`
	Kind         Kind    `json:"kind"`
	Name         string  `json:"name"`
	PixelW       int     `json:"pixelW"`
	PixelH       int     `json:"pixelH"`
	PPI          float64 `json:"ppi"`
	MaxHz        int     `json:"maxHz"`
	DefaultScale string  `json:"defaultScale"`
}

const (
	// DefaultPresetID is the iPad Pro 13" (M4), the iPad preset used by default.
	DefaultPresetID = "ipad-pro-13-m4"
	// DefaultPCPresetID is 1920×1080, the PC monitor preset used by default.
	DefaultPCPresetID = "pc-1080p"
)

// Scale options.
//
// iPad presets use Retina 2x relative scales (ScaleLarger/ScaleDefault/ScaleMore)
// plus 1:1 native. PC presets use desktop percentage scales (ScaleNative = 100%,
// Scale125, Scale150, Scale200).
const (
	ScaleLarger  = "larger"  // iPad: bigger UI, less space
	ScaleDefault = "default" // iPad: Retina 2x (pixels / 2); PC: preset's DefaultScale
	ScaleMore    = "more"    // iPad: more space, smaller UI
	ScaleNative  = "native"  // 1:1 pixels, no HiDPI (100% on PC)

	Scale125 = "125" // PC: 125% HiDPI
	Scale150 = "150" // PC: 150% HiDPI
	Scale200 = "200" // PC: 200% HiDPI (Retina 2x)
)

// Presets lists the supported devices. The first entry is the default iPad.
var Presets = []Preset{
	{ID: "ipad-pro-13-m4", Kind: KindIPad, Name: `iPad Pro 13" (M4)`, PixelW: 2752, PixelH: 2064, PPI: 264, MaxHz: 120, DefaultScale: ScaleDefault},
	{ID: "ipad-pro-11-m4", Kind: KindIPad, Name: `iPad Pro 11" (M4)`, PixelW: 2420, PixelH: 1668, PPI: 264, MaxHz: 120, DefaultScale: ScaleDefault},
	{ID: "ipad-pro-12.9", Kind: KindIPad, Name: `iPad Pro 12.9" (第 3-6 代)`, PixelW: 2732, PixelH: 2048, PPI: 264, MaxHz: 120, DefaultScale: ScaleDefault},
	{ID: "ipad-air-13", Kind: KindIPad, Name: `iPad Air 13" (M2/M3)`, PixelW: 2732, PixelH: 2048, PPI: 264, MaxHz: 60, DefaultScale: ScaleDefault},
	{ID: "ipad-air-11", Kind: KindIPad, Name: `iPad Air 11" (M2/M3)`, PixelW: 2360, PixelH: 1640, PPI: 264, MaxHz: 60, DefaultScale: ScaleDefault},
	{ID: "ipad-mini-a17", Kind: KindIPad, Name: "iPad mini (A17 Pro)", PixelW: 2266, PixelH: 1488, PPI: 326, MaxHz: 60, DefaultScale: ScaleDefault},

	{ID: "pc-1080p", Kind: KindPC, Name: "1920×1080 (1080p · 16:9)", PixelW: 1920, PixelH: 1080, PPI: 92, MaxHz: 120, DefaultScale: ScaleNative},
	{ID: "pc-1200p", Kind: KindPC, Name: "1920×1200 (16:10)", PixelW: 1920, PixelH: 1200, PPI: 94, MaxHz: 120, DefaultScale: ScaleNative},
	{ID: "pc-1440p", Kind: KindPC, Name: "2560×1440 (2K · 16:9)", PixelW: 2560, PixelH: 1440, PPI: 109, MaxHz: 120, DefaultScale: ScaleNative},
	{ID: "pc-1600p", Kind: KindPC, Name: "2560×1600 (2.5K · 16:10)", PixelW: 2560, PixelH: 1600, PPI: 189, MaxHz: 120, DefaultScale: Scale150},
	{ID: "pc-2160p", Kind: KindPC, Name: "3840×2160 (4K · 16:9)", PixelW: 3840, PixelH: 2160, PPI: 163, MaxHz: 120, DefaultScale: Scale200},
	{ID: "pc-ultrawide-1440p", Kind: KindPC, Name: "3440×1440 (21:9 带鱼屏)", PixelW: 3440, PixelH: 1440, PPI: 110, MaxHz: 120, DefaultScale: ScaleNative},
}

// Find returns the preset with the given id, or the default iPad preset.
func Find(id string) Preset {
	for _, p := range Presets {
		if p.ID == id {
			return p
		}
	}
	return Presets[0]
}

// DefaultPreset returns the default preset for the given device kind.
func DefaultPreset(kind Kind) Preset {
	if kind == KindPC {
		return Find(DefaultPCPresetID)
	}
	return Find(DefaultPresetID)
}

// PresetsByKind returns all presets belonging to kind.
func PresetsByKind(kind Kind) []Preset {
	var out []Preset
	for _, p := range Presets {
		if p.Kind == kind {
			out = append(out, p)
		}
	}
	return out
}

// ValidScale reports whether scale is a valid UI option for kind.
func ValidScale(kind Kind, scale string) bool {
	if kind == KindPC {
		switch scale {
		case ScaleNative, Scale125, Scale150, Scale200:
			return true
		}
		return false
	}
	switch scale {
	case ScaleLarger, ScaleDefault, ScaleMore, ScaleNative:
		return true
	}
	return false
}

// DefaultOptions returns the default options for a client device kind.
func DefaultOptions(kind Kind) Options {
	p := DefaultPreset(kind)
	return Options{
		PresetID:  p.ID,
		Scale:     p.DefaultScale,
		RefreshHz: 60,
	}
}

// NormalizeOptions fills in missing or invalid values for the given device kind.
func NormalizeOptions(o Options, kind Kind) Options {
	p := Find(o.PresetID)
	if p.ID != o.PresetID || p.Kind != kind {
		p = DefaultPreset(kind)
		o.PresetID = p.ID
	}
	if !ValidScale(kind, o.Scale) {
		if kind == KindPC && o.CustomW > 0 && o.CustomH > 0 {
			o.Scale = ScaleNative
		} else {
			o.Scale = p.DefaultScale
		}
	}
	if o.RefreshHz != 60 && o.RefreshHz != 120 {
		o.RefreshHz = 60
	}
	return o
}

var scaleFactors = map[string]float64{
	ScaleLarger:  0.875,
	ScaleDefault: 1.0,
	ScaleMore:    1.15,
}

var pcScaleModes = []struct {
	key   string
	div   float64
	hiDPI bool
}{
	{Scale200, 2.0, true},
	{Scale150, 1.5, true},
	{Scale125, 1.25, true},
	{ScaleNative, 1.0, false},
}

// Mode and Spec describe the virtual display; they live in package display,
// which creates it.
type (
	Mode = display.Mode
	Spec = display.Spec
)

// Options are the user-facing knobs for one client profile.
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
	custom := o.CustomW > 0 && o.CustomH > 0
	if custom {
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
	if custom && p.Kind == KindPC {
		ppi = 109
	} else if ppi <= 0 {
		if p.Kind == KindPC {
			ppi = 109
		} else {
			ppi = 264
		}
	}

	spec := Spec{
		Name:      "InfiWindow · " + p.Name,
		RefreshHz: hz,
		WidthMM:   math.Round(float64(pw)/ppi*25.4*10) / 10,
		HeightMM:  math.Round(float64(ph)/ppi*25.4*10) / 10,
	}
	if custom {
		spec.Name = fmt.Sprintf("InfiWindow · %d×%d", pw, ph)
	}

	if p.Kind == KindPC {
		defScale := p.DefaultScale
		if custom || defScale == "" {
			defScale = ScaleNative
		}
		sel := o.Scale
		if sel == "" || sel == ScaleDefault {
			sel = defScale
		}
		// Register desktop percentage scales (200%, 150%, 125% HiDPI and 100%
		// native 1:1) so the user can also switch from System Settings.
		for _, sm := range pcScaleModes {
			m := Mode{
				Width:  even(float64(pw) / sm.div),
				Height: even(float64(ph) / sm.div),
				HiDPI:  sm.hiDPI,
			}
			spec.Modes = append(spec.Modes, m)
			if sm.key == sel {
				spec.Selected = m
			}
		}
		if spec.Selected.Width == 0 {
			for i, sm := range pcScaleModes {
				if sm.key == defScale {
					spec.Selected = spec.Modes[i]
					break
				}
			}
			if spec.Selected.Width == 0 {
				spec.Selected = spec.Modes[len(spec.Modes)-1]
			}
		}
	} else {
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
	}

	for _, m := range spec.Modes {
		w, h := m.PixelSize()
		spec.MaxPixelW = max(spec.MaxPixelW, w)
		spec.MaxPixelH = max(spec.MaxPixelH, h)
	}
	return spec, nil
}
