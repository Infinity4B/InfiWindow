//go:build manual

// Run with: CGO_ENABLED=0 go test -tags manual -run Manual -v ./internal/vdisplay
// It briefly adds a real virtual display to this Mac.
package vdisplay

import (
	"os"
	"testing"
	"time"

	"github.com/Infinity4B/infiwindow/internal/device"
)

func TestManualCreate(t *testing.T) {
	spec, err := device.BuildSpec(device.Options{PresetID: device.DefaultPresetID, Scale: device.ScaleDefault, RefreshHz: 60})
	if err != nil {
		t.Fatal(err)
	}
	layout := Layout(os.Getenv("IW_LAYOUT"))
	if layout == "" {
		layout = LayoutExtend
	}
	m := NewManager()
	id, err := m.Create(spec, layout)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Destroy()
	t.Logf("created display %d", id)
	for _, d := range m.ListDisplays() {
		t.Logf("%+v", d)
	}
	var found bool
	for _, d := range m.ListDisplays() {
		if d.ID == id {
			found = true
			if d.Width != 1376 || d.Height != 1032 || d.PixelWidth != 2752 || d.PixelHeight != 2064 {
				t.Errorf("unexpected mode %+v", d)
			}
		}
	}
	if !found {
		t.Fatal("virtual display not listed")
	}
	time.Sleep(time.Second)
	m.Destroy()
	if layout != LayoutExtend {
		// Without a run loop this process never hears that the mirror set
		// changed, so its display list goes stale; check from another
		// process (-run ManualList) instead.
		return
	}
	start := time.Now()
	for {
		gone := true
		for _, d := range ListDisplays() {
			if d.ID == id {
				gone = false
			}
		}
		if gone {
			t.Logf("removed after %v", time.Since(start))
			break
		}
		if time.Since(start) > 8*time.Second {
			t.Fatal("display still online after Destroy")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
