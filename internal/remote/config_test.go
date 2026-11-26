package remote

import (
	"testing"

	"github.com/Infinity4B/infiwindow/internal/remote/device"
)

func TestDefaultConfigProfiles(t *testing.T) {
	c := DefaultConfig()
	if c.Active != device.KindIPad {
		t.Fatalf("active = %q, want ipad", c.Active)
	}
	if c.Profiles.IPad.PresetID != device.DefaultPresetID || c.Profiles.IPad.Scale != device.ScaleDefault {
		t.Fatalf("ipad defaults = %+v", c.Profiles.IPad)
	}
	if c.Profiles.PC.PresetID != device.DefaultPCPresetID || c.Profiles.PC.Scale != device.ScaleNative {
		t.Fatalf("pc defaults = %+v", c.Profiles.PC)
	}
	if c.ActiveDevice() != c.Profiles.IPad {
		t.Fatal("ActiveDevice should return iPad profile")
	}
	c.Active = device.KindPC
	if c.ActiveDevice() != c.Profiles.PC {
		t.Fatal("ActiveDevice should return PC profile")
	}
	c.ActiveDevicePtr().RefreshHz = 120
	if c.Profiles.PC.RefreshHz != 120 {
		t.Fatal("ActiveDevicePtr did not mutate PC profile")
	}
}

func TestNormalizeMigratesLegacyDevice(t *testing.T) {
	legacy := Config{
		Device: device.Options{
			PresetID:  "ipad-air-11",
			Portrait:  true,
			Scale:     device.ScaleMore,
			RefreshHz: 60,
			CustomW:   2000,
			CustomH:   1500,
		},
	}
	got := legacy.Normalize()
	if got.Active != device.KindIPad {
		t.Fatalf("active = %q, want ipad", got.Active)
	}
	if got.Device != (device.Options{}) {
		t.Fatalf("legacy Device should be cleared: %+v", got.Device)
	}
	if got.Profiles.IPad != legacy.Device {
		t.Fatalf("migrated iPad = %+v, want %+v", got.Profiles.IPad, legacy.Device)
	}
	if got.Profiles.PC != device.DefaultOptions(device.KindPC) {
		t.Fatalf("PC profile = %+v, want defaults", got.Profiles.PC)
	}
}

func TestNormalizePreservesProfilesOverLegacyDevice(t *testing.T) {
	cfg := Config{
		Active: device.KindPC,
		Profiles: Profiles{
			IPad: device.Options{PresetID: "ipad-mini-a17", Scale: device.ScaleLarger, RefreshHz: 60},
			PC:   device.Options{PresetID: "pc-2160p", Scale: device.Scale200, RefreshHz: 120},
		},
		Device: device.Options{PresetID: "ipad-air-11", Scale: device.ScaleMore, RefreshHz: 60},
	}
	got := cfg.Normalize()
	if got.Active != device.KindPC {
		t.Fatalf("active = %q", got.Active)
	}
	if got.Profiles.IPad.PresetID != "ipad-mini-a17" || got.Profiles.PC.PresetID != "pc-2160p" {
		t.Fatalf("profiles overwritten: %+v", got.Profiles)
	}
	if got.Device != (device.Options{}) {
		t.Fatalf("legacy Device not cleared: %+v", got.Device)
	}
}
