// Package hidpi adds HiDPI ("looks like") resolutions to non-Retina external
// monitors with a display override file, the way macOS describes its own
// panels under /System/Library/Displays/.../Overrides.
//
// The file lists scaled resolutions in points; macOS then offers each one
// backed by 2x pixels, renders at that size and scales the result down to
// the panel. It is read when the monitor connects, so a new file takes
// effect after the monitor is plugged in again (or the Mac restarts).
// Nothing has to keep running: removing the file undoes it.
package hidpi

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Root holds user-installed display overrides. Unlike the /System copy it is
// not protected by SIP, but writing to it needs administrator rights.
const Root = "/Library/Displays/Contents/Resources/Overrides"

// marker is a key InfiWindow adds to the files it writes, so it never
// replaces or deletes an override made by hand or by another tool.
const marker = "InfiWindowManaged"

// Size is a resolution in points.
type Size struct {
	W, H int
}

func (s Size) String() string { return fmt.Sprintf("%d×%d", s.W, s.H) }

// Option is one scaled resolution offered for a monitor.
type Option struct {
	Size
	// Percent is how much larger the UI looks than at the native 1:1 mode.
	Percent int
}

// scales are the UI magnifications offered, as native/points ratios.
// 200% is left out: macOS already offers it (native pixels at 2x).
var scales = []struct{ num, den int }{
	{10, 9}, // 111%
	{5, 4},  // 125%
	{4, 3},  // 133%
	{3, 2},  // 150%
	{8, 5},  // 160%
	{7, 4},  // 175%
}

// even rounds to the nearest whole number, then up to an even one.
func even(v float64) int {
	n := int(math.Round(v))
	if n%2 != 0 {
		n++
	}
	return n
}

// Options lists the scaled resolutions for a panel of nativeW×nativeH pixels,
// from the most space to the largest UI.
func Options(nativeW, nativeH int) []Option {
	var out []Option
	for _, s := range scales {
		f := float64(s.den) / float64(s.num)
		sz := Size{W: even(float64(nativeW) * f), H: even(float64(nativeH) * f)}
		if sz.W < 640 || sz.H < 480 {
			continue
		}
		out = append(out, Option{Size: sz, Percent: int(math.Round(float64(s.num) / float64(s.den) * 100))})
	}
	return out
}

// Path is the override file for a monitor, named the way macOS looks it up:
// lowercase hex IDs without leading zeros.
func Path(vendor, product uint32) string {
	return filepath.Join(Root, fmt.Sprintf("DisplayVendorID-%x", vendor), fmt.Sprintf("DisplayProductID-%x", product))
}

// Build returns the override property list. Each scaled resolution is 12
// bytes: width and height in points, then flags, where 1 asks for a HiDPI
// (2x) mode. That is the layout of Apple's own override files.
func Build(vendor, product uint32, sizes []Size) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	fmt.Fprintf(&b, "\t<key>DisplayProductID</key>\n\t<integer>%d</integer>\n", product)
	fmt.Fprintf(&b, "\t<key>DisplayVendorID</key>\n\t<integer>%d</integer>\n", vendor)
	fmt.Fprintf(&b, "\t<key>%s</key>\n\t<true/>\n", marker)
	b.WriteString("\t<key>scale-resolutions</key>\n\t<array>\n")
	for _, s := range sizes {
		var e [12]byte
		binary.BigEndian.PutUint32(e[0:], uint32(s.W))
		binary.BigEndian.PutUint32(e[4:], uint32(s.H))
		binary.BigEndian.PutUint32(e[8:], 1)
		fmt.Fprintf(&b, "\t\t<data>%s</data>\n", base64.StdEncoding.EncodeToString(e[:]))
	}
	b.WriteString("\t</array>\n</dict>\n</plist>\n")
	return b.Bytes()
}

// Override is what is installed for one monitor.
type Override struct {
	Exists bool
	// Managed says InfiWindow wrote the file and may replace or remove it.
	Managed bool
	// Sizes are the HiDPI resolutions it lists.
	Sizes []Size
}

var (
	scaleArray = regexp.MustCompile(`(?s)<key>scale-resolutions</key>\s*<array>(.*?)</array>`)
	dataEntry  = regexp.MustCompile(`(?s)<data>(.*?)</data>`)
)

// Parse reads an override file in XML form. Binary property lists (which
// InfiWindow never writes) are reported as existing but unmanaged.
func Parse(b []byte) Override {
	o := Override{Exists: true, Managed: bytes.Contains(b, []byte("<key>"+marker+"</key>"))}
	m := scaleArray.FindSubmatch(b)
	if m == nil {
		return o
	}
	for _, d := range dataEntry.FindAllSubmatch(m[1], -1) {
		raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(d[1])), ""))
		if err != nil || len(raw) < 12 {
			continue
		}
		if binary.BigEndian.Uint32(raw[8:])&1 == 0 {
			continue
		}
		o.Sizes = append(o.Sizes, Size{W: int(binary.BigEndian.Uint32(raw[0:])), H: int(binary.BigEndian.Uint32(raw[4:]))})
	}
	return o
}

// Read returns the override installed for a monitor.
func Read(vendor, product uint32) (Override, error) {
	b, err := os.ReadFile(Path(vendor, product))
	if errors.Is(err, os.ErrNotExist) {
		return Override{}, nil
	}
	if err != nil {
		return Override{}, err
	}
	return Parse(b), nil
}

// ErrForeign is returned when an override InfiWindow did not write is in the way.
var ErrForeign = errors.New("这台显示器已有其他工具创建的覆盖文件，InfiWindow 不会修改它")

// ErrCanceled is returned when the user dismisses the password prompt.
var ErrCanceled = errors.New("已取消")
