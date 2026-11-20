package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Infinity4B/infiwindow/internal/remote"
	"github.com/Infinity4B/infiwindow/internal/remote/device"
)

func TestRoundTripAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Remote.Device.PresetID != device.DefaultPresetID || c.Remote.Mode != remote.ModeAuto {
		t.Fatalf("bad defaults %+v", c)
	}
	c.Remote.Device.Portrait = true
	c.Remote.Device.RefreshHz = 120
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Remote.Device.Portrait || got.Remote.Device.RefreshHz != 120 {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

func TestNormalizeRepairsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"remote":{"device":{"presetId":"nope","scale":"huge","refreshHz":75},"layout":"x","mode":"y","processes":["  "]}}`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d, r := remote.DefaultConfig(), c.Remote
	if r.Device.PresetID != d.Device.PresetID || r.Device.Scale != d.Device.Scale || r.Device.RefreshHz != 60 ||
		r.Layout != d.Layout || r.Mode != d.Mode || len(r.Processes) != 1 {
		t.Fatalf("not repaired: %+v", r)
	}
}

// Settings saved by 0.1.x sit at the top level.
func TestLoadLegacyFlatFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"device":{"presetId":"ipad-air-11","scale":"more","refreshHz":60},"layout":"extend","mode":"manual","processes":["Foo"]}`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r := c.Remote
	if r.Device.PresetID != "ipad-air-11" || r.Device.Scale != device.ScaleMore || r.Layout != "extend" ||
		r.Mode != remote.ModeManual || len(r.Processes) != 1 || r.Processes[0] != "Foo" {
		t.Fatalf("legacy settings lost: %+v", r)
	}
}
