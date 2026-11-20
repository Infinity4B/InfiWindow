package hidpi

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestOptions1440p(t *testing.T) {
	got := Options(2560, 1440)
	want := []Option{
		{Size{2304, 1296}, 111},
		{Size{2048, 1152}, 125},
		{Size{1920, 1080}, 133},
		{Size{1708, 960}, 150},
		{Size{1600, 900}, 160},
		{Size{1464, 824}, 175},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Options(2560, 1440) =\n%v\nwant\n%v", got, want)
	}
}

func TestOptionsSkipTiny(t *testing.T) {
	for _, o := range Options(1024, 768) {
		if o.W < 640 || o.H < 480 {
			t.Fatalf("too small: %v", o)
		}
	}
}

func TestPath(t *testing.T) {
	want := "/Library/Displays/Contents/Resources/Overrides/DisplayVendorID-5e3/DisplayProductID-2703"
	if got := Path(0x5e3, 0x2703); got != want {
		t.Fatalf("Path = %s", got)
	}
}

func TestBuildParseRoundTrip(t *testing.T) {
	sizes := []Size{{1920, 1080}, {1708, 960}}
	b := Build(1507, 9987, sizes)
	o := Parse(b)
	if !o.Exists || !o.Managed || !reflect.DeepEqual(o.Sizes, sizes) {
		t.Fatalf("Parse(Build) = %+v", o)
	}
	if runtime.GOOS == "darwin" {
		f := filepath.Join(t.TempDir(), "o.plist")
		if err := os.WriteFile(f, b, 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("plutil", "-lint", f).CombinedOutput(); err != nil {
			t.Fatalf("plutil: %v %s", err, out)
		}
	}
}

// Apple's overrides wrap base64 over several lines; files from other tools
// lack the marker and must be left alone.
func TestParseForeign(t *testing.T) {
	b := []byte(`<plist><dict>
	<key>scale-resolutions</key>
	<array>
		<data>
		AAAHgAAABDgAAAAB
		</data>
		<data>AAAPAAAACHAAAAAAAAAAAA==</data>
	</array></dict></plist>`)
	o := Parse(b)
	if o.Managed {
		t.Fatal("foreign file reported as managed")
	}
	if want := []Size{{1920, 1080}}; !reflect.DeepEqual(o.Sizes, want) {
		t.Fatalf("sizes = %v, want %v (the 1x entry is skipped)", o.Sizes, want)
	}
}

func TestQuote(t *testing.T) {
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Fatal(got)
	}
	if got := appleScriptQuote(`say "hi" \ ok`); got != `"say \"hi\" \\ ok"` {
		t.Fatal(got)
	}
}
