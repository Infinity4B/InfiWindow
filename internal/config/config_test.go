package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Infinity4B/infiwindow/internal/device"
)

func TestRoundTripAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Device.PresetID != device.DefaultPresetID || c.Mode != ModeAuto {
		t.Fatalf("bad defaults %+v", c)
	}
	c.Device.Portrait = true
	c.Device.RefreshHz = 120
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Device.Portrait || got.Device.RefreshHz != 120 {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

func TestNormalizeRepairsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"device":{"presetId":"nope","scale":"huge","refreshHz":75},"layout":"x","mode":"y","processes":["  "]}`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d := Default()
	if c.Device.PresetID != d.Device.PresetID || c.Device.Scale != d.Device.Scale || c.Device.RefreshHz != 60 ||
		c.Layout != d.Layout || c.Mode != d.Mode || len(c.Processes) != 1 {
		t.Fatalf("not repaired: %+v", c)
	}
}
