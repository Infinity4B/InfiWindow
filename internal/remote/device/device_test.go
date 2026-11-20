package device

import "testing"

func TestDefaultIsIPadPro13M4(t *testing.T) {
	s, err := BuildSpec(Options{PresetID: DefaultPresetID, Scale: ScaleDefault, RefreshHz: 60})
	if err != nil {
		t.Fatal(err)
	}
	if s.Selected != (Mode{Width: 1376, Height: 1032, HiDPI: true}) {
		t.Fatalf("selected = %+v", s.Selected)
	}
	if w, h := s.Selected.PixelSize(); w != 2752 || h != 2064 {
		t.Fatalf("pixels = %dx%d", w, h)
	}
	if s.WidthMM < 264 || s.WidthMM > 266 || s.HeightMM < 198 || s.HeightMM > 199.5 {
		t.Fatalf("size = %vx%v mm", s.WidthMM, s.HeightMM)
	}
	if s.MaxPixelW < 2752 || s.MaxPixelH < 2064 {
		t.Fatalf("max pixels too small: %dx%d", s.MaxPixelW, s.MaxPixelH)
	}
}

func TestPortraitAndNative(t *testing.T) {
	s, _ := BuildSpec(Options{PresetID: DefaultPresetID, Portrait: true, Scale: ScaleNative, RefreshHz: 120})
	if s.Selected != (Mode{Width: 2064, Height: 2752}) {
		t.Fatalf("selected = %+v", s.Selected)
	}
	if s.RefreshHz != 120 {
		t.Fatalf("hz = %v", s.RefreshHz)
	}
}

func TestEveryModeFitsMaxPixels(t *testing.T) {
	for _, p := range Presets {
		for _, sc := range []string{ScaleLarger, ScaleDefault, ScaleMore, ScaleNative} {
			s, err := BuildSpec(Options{PresetID: p.ID, Scale: sc})
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range s.Modes {
				w, h := m.PixelSize()
				if w > s.MaxPixelW || h > s.MaxPixelH || m.Width%2 != 0 || m.Height%2 != 0 {
					t.Fatalf("%s/%s bad mode %+v (max %dx%d)", p.ID, sc, m, s.MaxPixelW, s.MaxPixelH)
				}
			}
		}
	}
}

func TestCustomValidation(t *testing.T) {
	if _, err := BuildSpec(Options{CustomW: 100, CustomH: 100}); err == nil {
		t.Fatal("expected error")
	}
	s, err := BuildSpec(Options{CustomW: 1668, CustomH: 2420, Scale: ScaleDefault})
	if err != nil {
		t.Fatal(err)
	}
	if s.Selected.Width != 1210 || s.Selected.Height != 834 {
		t.Fatalf("custom should normalise to landscape: %+v", s.Selected)
	}
}
