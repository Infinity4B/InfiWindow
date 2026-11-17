// Package config stores InfiWindow's settings as JSON.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Infinity4B/infiwindow/internal/detector"
	"github.com/Infinity4B/infiwindow/internal/device"
	"github.com/Infinity4B/infiwindow/internal/vdisplay"
)

// Mode says what drives the virtual display.
type Mode string

const (
	ModeAuto   Mode = "auto"   // follow UU Remote sessions
	ModeManual Mode = "manual" // only the buttons
)

// Config is everything the user can change.
type Config struct {
	Device device.Options  `json:"device"`
	Layout vdisplay.Layout `json:"layout"`
	Mode   Mode            `json:"mode"`
	// Processes are name prefixes of the remote-desktop processes to watch.
	Processes []string        `json:"processes"`
	Detect    detector.Params `json:"detect"`
}

// Default targets an iPad Pro 13" (M4) over UU Remote.
func Default() Config {
	return Config{
		Device: device.Options{
			PresetID:  device.DefaultPresetID,
			Scale:     device.ScaleDefault,
			RefreshHz: 60,
		},
		Layout:    vdisplay.LayoutMirror,
		Mode:      ModeAuto,
		Processes: []string{"UURemote"},
		Detect:    detector.DefaultParams(),
	}
}

// Normalize fills in missing or invalid values with defaults.
func (c Config) Normalize() Config {
	d := Default()
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
	case vdisplay.LayoutMirror, vdisplay.LayoutMain, vdisplay.LayoutExtend:
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

// Load reads path, returning defaults when it does not exist.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	c := Default()
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), err
	}
	return c.Normalize(), nil
}

// Save writes c to path atomically.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
