//go:build genicon

// Regenerates resources/icon.png and assets/icon.svg's raster twin:
//
//	CGO_ENABLED=0 go test -tags genicon -run GenIcon .
package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestGenIcon(t *testing.T) {
	img := ui.Render(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { drawAppIcon(p, r) })
	}, 1024, 1024, 1)
	f, err := os.Create("resources/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// drawAppIcon follows macOS's icon grid: an 824-pt rounded square centred in
// 1024 with room for the shadow.
func drawAppIcon(p *ui.Painter, r ui.Rect) {
	s := r.W / 1024
	sq := ui.Rect{X: r.X + 100*s, Y: r.Y + 100*s, W: 824 * s, H: 824 * s}
	var bg ui.Path
	roundRect(&bg, sq, 185*s)
	p.FillPathGradient(&bg, ui.LinearGradient{From: ui.Hex("#6AA8FF"), To: ui.Hex("#2A4FE0"), Angle: 160, Oklab: true})

	// The iPad, landscape, in white.
	pad := ui.Rect{X: sq.X + 150*s, Y: sq.Y + 230*s, W: 524 * s, H: 364 * s}
	var body ui.Path
	roundRect(&body, pad, 46*s)
	p.FillPath(&body, ui.Hex("#FFFFFF"))
	// Its screen, showing a window that fits it edge to edge.
	scr := ui.Rect{X: pad.X + 26*s, Y: pad.Y + 26*s, W: pad.W - 52*s, H: pad.H - 52*s}
	var screen ui.Path
	roundRect(&screen, scr, 22*s)
	p.FillPathGradient(&screen, ui.LinearGradient{From: ui.Hex("#DCE9FF"), To: ui.Hex("#B5CCFF"), Angle: 180})
	// Title bar and three dots of the "window".
	p.Fill(ui.Rect{X: scr.X, Y: scr.Y, W: scr.W, H: 44 * s}, ui.Hex("#8FB2FF"), 0)
	for i, col := range []string{"#FF6B6B", "#FFC94D", "#4CD07D"} {
		cx := scr.X + (34+float32(i)*34)*s
		var dot ui.Path
		dot.Circle(cx, scr.Y+22*s, 10*s)
		p.FillPath(&dot, ui.Hex(col))
	}
	// Infinity loop on the screen.
	var inf ui.Path
	cx, cy := scr.X+scr.W/2, scr.Y+44*s+(scr.H-44*s)/2
	w, h := 92*s, 52*s
	inf.MoveTo(cx, cy).
		CubeTo(cx+w*0.35, cy-h, cx+w, cy-h, cx+w, cy).
		CubeTo(cx+w, cy+h, cx+w*0.35, cy+h, cx, cy).
		CubeTo(cx-w*0.35, cy-h, cx-w, cy-h, cx-w, cy).
		CubeTo(cx-w, cy+h, cx-w*0.35, cy+h, cx, cy)
	p.StrokePath(&inf, 22*s, ui.Hex("#2A4FE0"))
}

func roundRect(path *ui.Path, r ui.Rect, rad float32) {
	k := float32(0.5523) * rad
	path.MoveTo(r.X+rad, r.Y).
		LineTo(r.X+r.W-rad, r.Y).
		CubeTo(r.X+r.W-rad+k, r.Y, r.X+r.W, r.Y+rad-k, r.X+r.W, r.Y+rad).
		LineTo(r.X+r.W, r.Y+r.H-rad).
		CubeTo(r.X+r.W, r.Y+r.H-rad+k, r.X+r.W-rad+k, r.Y+r.H, r.X+r.W-rad, r.Y+r.H).
		LineTo(r.X+rad, r.Y+r.H).
		CubeTo(r.X+rad-k, r.Y+r.H, r.X, r.Y+r.H-rad+k, r.X, r.Y+r.H-rad).
		LineTo(r.X, r.Y+rad).
		CubeTo(r.X, r.Y+rad-k, r.X+rad-k, r.Y, r.X+rad, r.Y).
		Close()
}
