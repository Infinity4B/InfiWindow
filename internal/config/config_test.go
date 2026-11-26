package config

import (
	"os"
	"path/filepath"
	"strings"
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
	if c.Remote.Active != device.KindIPad ||
		c.Remote.Profiles.IPad.PresetID != device.DefaultPresetID ||
		c.Remote.Profiles.PC.PresetID != device.DefaultPCPresetID ||
		c.Remote.Mode != remote.ModeAuto {
		t.Fatalf("bad defaults %+v", c)
	}
	c.Remote.Active = device.KindPC
	c.Remote.Profiles.IPad.Portrait = true
	c.Remote.Profiles.IPad.RefreshHz = 120
	c.Remote.Profiles.PC.PresetID = "pc-2160p"
	c.Remote.Profiles.PC.Scale = device.Scale200
	c.Remote.Profiles.PC.RefreshHz = 120
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"device"`) {
		t.Fatalf("saved config should omit legacy device field: %s", raw)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote.Active != device.KindPC ||
		!got.Remote.Profiles.IPad.Portrait || got.Remote.Profiles.IPad.RefreshHz != 120 ||
		got.Remote.Profiles.PC.PresetID != "pc-2160p" || got.Remote.Profiles.PC.Scale != device.Scale200 || got.Remote.Profiles.PC.RefreshHz != 120 {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

func TestNormalizeRepairsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"remote":{"active":"tv","profiles":{"ipad":{"presetId":"pc-1080p","scale":"huge","refreshHz":75},"pc":{"presetId":"nope","scale":"more","refreshHz":90}},"layout":"x","mode":"y","processes":["  "]}}`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d, r := remote.DefaultConfig(), c.Remote
	if r.Active != d.Active ||
		r.Profiles.IPad != d.Profiles.IPad ||
		r.Profiles.PC != d.Profiles.PC ||
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
	if r.Active != device.KindIPad ||
		r.Profiles.IPad.PresetID != "ipad-air-11" || r.Profiles.IPad.Scale != device.ScaleMore ||
		r.Profiles.PC != device.DefaultOptions(device.KindPC) ||
		r.Layout != "extend" || r.Mode != remote.ModeManual || len(r.Processes) != 1 || r.Processes[0] != "Foo" {
		t.Fatalf("legacy settings lost: %+v", r)
	}
}

// Settings saved before dual iPad/PC profiles had only remote.device.
func TestLoadLegacyRemoteDeviceFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"remote":{"device":{"presetId":"ipad-pro-11-m4","portrait":true,"scale":"larger","refreshHz":120,"customW":2200,"customH":1600},"layout":"main","mode":"auto","processes":["UURemote"]}}`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r := c.Remote
	wantIPad := device.Options{
		PresetID:  "ipad-pro-11-m4",
		Portrait:  true,
		Scale:     device.ScaleLarger,
		RefreshHz: 120,
		CustomW:   2200,
		CustomH:   1600,
	}
	if r.Active != device.KindIPad ||
		r.Profiles.IPad != wantIPad ||
		r.Profiles.PC != device.DefaultOptions(device.KindPC) ||
		r.Device != (device.Options{}) ||
		r.Layout != "main" || r.Mode != remote.ModeAuto {
		t.Fatalf("legacy remote.device migration failed: %+v", r)
	}
}
