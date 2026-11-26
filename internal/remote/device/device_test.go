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
		scales := []string{ScaleLarger, ScaleDefault, ScaleMore, ScaleNative}
		if p.Kind == KindPC {
			scales = []string{ScaleDefault, ScaleNative, Scale125, Scale150, Scale200}
		}
		for _, sc := range scales {
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

func TestPCPresetsBuildSpec(t *testing.T) {
	cases := []struct {
		id         string
		wantMode   Mode
		wantMinWMM float64
		wantMaxWMM float64
	}{
		{"pc-1080p", Mode{Width: 1920, Height: 1080, HiDPI: false}, 529, 531},
		{"pc-1200p", Mode{Width: 1920, Height: 1200, HiDPI: false}, 518, 520},
		{"pc-1440p", Mode{Width: 2560, Height: 1440, HiDPI: false}, 595, 598},
		{"pc-1600p", Mode{Width: 1708, Height: 1068, HiDPI: true}, 343, 345},
		{"pc-2160p", Mode{Width: 1920, Height: 1080, HiDPI: true}, 597, 600},
		{"pc-ultrawide-1440p", Mode{Width: 3440, Height: 1440, HiDPI: false}, 793, 796},
	}
	for _, tc := range cases {
		s, err := BuildSpec(Options{PresetID: tc.id, Scale: ScaleDefault, RefreshHz: 60})
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if s.Selected != tc.wantMode {
			t.Fatalf("%s: selected = %+v, want %+v", tc.id, s.Selected, tc.wantMode)
		}
		if s.WidthMM < tc.wantMinWMM || s.WidthMM > tc.wantMaxWMM {
			t.Fatalf("%s: widthMM = %v, want in [%v, %v]", tc.id, s.WidthMM, tc.wantMinWMM, tc.wantMaxWMM)
		}
	}
}

func TestPCScalesPortraitAndCustom(t *testing.T) {
	s125, err := BuildSpec(Options{PresetID: "pc-1080p", Scale: Scale125, RefreshHz: 120})
	if err != nil {
		t.Fatal(err)
	}
	if s125.Selected != (Mode{Width: 1536, Height: 864, HiDPI: true}) || s125.RefreshHz != 120 {
		t.Fatalf("125%% = %+v @%v", s125.Selected, s125.RefreshHz)
	}

	port, err := BuildSpec(Options{PresetID: "pc-1080p", Portrait: true, Scale: ScaleNative})
	if err != nil {
		t.Fatal(err)
	}
	if port.Selected != (Mode{Width: 1080, Height: 1920, HiDPI: false}) {
		t.Fatalf("portrait PC = %+v", port.Selected)
	}

	cust, err := BuildSpec(Options{PresetID: DefaultPCPresetID, CustomW: 2560, CustomH: 1080})
	if err != nil {
		t.Fatal(err)
	}
	if cust.Selected != (Mode{Width: 2560, Height: 1080, HiDPI: false}) {
		t.Fatalf("custom PC selected = %+v", cust.Selected)
	}
	if cust.Name != "InfiWindow · 2560×1080" {
		t.Fatalf("custom PC name = %q", cust.Name)
	}
	// Custom PC uses 109 PPI, not iPad's 264 PPI (~246 mm).
	if cust.WidthMM < 595 || cust.WidthMM > 598 {
		t.Fatalf("custom PC widthMM = %v, want desktop ~596.6 mm", cust.WidthMM)
	}
}

func TestNormalizeOptionsAndPresetsByKind(t *testing.T) {
	if len(PresetsByKind(KindIPad)) == 0 || len(PresetsByKind(KindPC)) == 0 {
		t.Fatal("expected presets for both iPad and PC")
	}
	// Mismatched preset kind and invalid scale/hz should be repaired.
	pc := NormalizeOptions(Options{PresetID: DefaultPresetID, Scale: ScaleMore, RefreshHz: 75}, KindPC)
	if pc.PresetID != DefaultPCPresetID || pc.Scale != ScaleNative || pc.RefreshHz != 60 {
		t.Fatalf("normalized PC = %+v", pc)
	}
	// Custom PC with invalid scale defaults to 100% native even if preset is 4K.
	pcCustom := NormalizeOptions(Options{PresetID: "pc-2160p", CustomW: 1920, CustomH: 1080, Scale: ScaleDefault}, KindPC)
	if pcCustom.PresetID != "pc-2160p" || pcCustom.Scale != ScaleNative {
		t.Fatalf("normalized custom PC = %+v", pcCustom)
	}
	ipad := NormalizeOptions(Options{PresetID: "pc-1080p", Scale: Scale150, RefreshHz: 144}, KindIPad)
	if ipad.PresetID != DefaultPresetID || ipad.Scale != ScaleDefault || ipad.RefreshHz != 60 {
		t.Fatalf("normalized iPad = %+v", ipad)
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
