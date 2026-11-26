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

// Profiles stores independent virtual-display settings for each client kind.
type Profiles struct {
	IPad device.Options `json:"ipad"`
	PC   device.Options `json:"pc"`
}

// Config is everything the user can change about the remote display.
type Config struct {
	// Active selects which client profile (ipad or pc) drives the virtual display.
	Active device.Kind `json:"active"`
	// Profiles holds one device configuration each for iPad and PC clients.
	Profiles Profiles `json:"profiles"`
	// Device is the legacy single-device (iPad) field from older config files.
	// Normalize migrates it into Profiles.IPad when Profiles.IPad is empty.
	Device device.Options `json:"device,omitzero"`

	Layout display.Layout `json:"layout"`
	Mode   Mode           `json:"mode"`
	// Processes are name prefixes of the remote-desktop processes to watch.
	Processes []string        `json:"processes"`
	Detect    detector.Params `json:"detect"`
}

// DefaultConfig targets an iPad Pro 13" (M4) by default and ships with a
// ready-to-use PC profile for desktop monitors over UU Remote.
func DefaultConfig() Config {
	return Config{
		Active: device.KindIPad,
		Profiles: Profiles{
			IPad: device.DefaultOptions(device.KindIPad),
			PC:   device.DefaultOptions(device.KindPC),
		},
		Layout:    display.LayoutMirror,
		Mode:      ModeAuto,
		Processes: []string{"UURemote"},
		Detect:    detector.DefaultParams(),
	}
}

// ActiveDevice returns the options for the currently selected client profile.
func (c Config) ActiveDevice() device.Options {
	if c.Active == device.KindPC {
		return c.Profiles.PC
	}
	return c.Profiles.IPad
}

// ActiveDevicePtr returns a pointer to the currently selected client profile's
// options, for UI editing.
func (c *Config) ActiveDevicePtr() *device.Options {
	if c.Active == device.KindPC {
		return &c.Profiles.PC
	}
	return &c.Profiles.IPad
}

// Normalize fills in missing or invalid values with defaults and migrates
// legacy single-device configs into Profiles.IPad.
func (c Config) Normalize() Config {
	d := DefaultConfig()
	if c.Active != device.KindIPad && c.Active != device.KindPC {
		c.Active = d.Active
	}
	// Migrate legacy config files that only had the single "device" field.
	if c.Profiles.IPad == (device.Options{}) && c.Device != (device.Options{}) {
		c.Profiles.IPad = c.Device
	}
	c.Device = device.Options{}
	c.Profiles.IPad = device.NormalizeOptions(c.Profiles.IPad, device.KindIPad)
	c.Profiles.PC = device.NormalizeOptions(c.Profiles.PC, device.KindPC)

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
