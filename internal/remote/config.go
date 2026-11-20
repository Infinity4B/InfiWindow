package remote

import (
	"strings"

	"github.com/Infinity4B/infiwindow/internal/display"
	"github.com/Infinity4B/infiwindow/internal/remote/detector"
	"github.com/Infinity4B/infiwindow/internal/remote/device"
)

// Mode says what drives the virtual display.
type Mode string

const (
	ModeAuto   Mode = "auto"   // follow remote sessions
	ModeManual Mode = "manual" // only the buttons
)

// Config is everything the user can change about the remote display.
type Config struct {
	Device device.Options `json:"device"`
	Layout display.Layout `json:"layout"`
	Mode   Mode           `json:"mode"`
	// Processes are name prefixes of the remote-desktop processes to watch.
	Processes []string        `json:"processes"`
	Detect    detector.Params `json:"detect"`
}

// DefaultConfig targets an iPad Pro 13" (M4) over UU Remote.
func DefaultConfig() Config {
	return Config{
		Device: device.Options{
			PresetID:  device.DefaultPresetID,
			Scale:     device.ScaleDefault,
			RefreshHz: 60,
		},
		Layout:    display.LayoutMirror,
		Mode:      ModeAuto,
		Processes: []string{"UURemote"},
		Detect:    detector.DefaultParams(),
	}
}

// Normalize fills in missing or invalid values with defaults.
func (c Config) Normalize() Config {
	d := DefaultConfig()
	if device.Find(c.Device.PresetID).ID != c.Device.PresetID {
		c.Device.PresetID = d.Device.PresetID
	}
	switch c.Device.Scale {
	case device.ScaleLarger, device.ScaleDefault, device.ScaleMore, device.ScaleNative:
	default:
		c.Device.Scale = d.Device.Scale
	}
	if c.Device.RefreshHz != 60 && c.Device.RefreshHz != 120 {
		c.Device.RefreshHz = 60
	}
	switch c.Layout {
	case display.LayoutMirror, display.LayoutMain, display.LayoutExtend:
	default:
		c.Layout = d.Layout
	}
	if c.Mode != ModeAuto && c.Mode != ModeManual {
		c.Mode = d.Mode
	}
	var procs []string
	for _, p := range c.Processes {
		if p = strings.TrimSpace(p); p != "" {
			procs = append(procs, p)
		}
	}
	if len(procs) == 0 {
		procs = d.Processes
	}
	c.Processes = procs
	c.Detect = c.Detect.Normalize()
	return c
}
