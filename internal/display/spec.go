package display

// Mode is one mode offered for a virtual display. Width/Height are in
// points; HiDPI modes are backed by 2x pixels.
type Mode struct {
	Width  int  `json:"width"`
	Height int  `json:"height"`
	HiDPI  bool `json:"hiDPI"`
}

// PixelSize returns the backing pixel size of the mode.
func (m Mode) PixelSize() (int, int) {
	if m.HiDPI {
		return m.Width * 2, m.Height * 2
	}
	return m.Width, m.Height
}

// Spec is everything needed to create a virtual display.
type Spec struct {
	Name      string  `json:"name"`
	Selected  Mode    `json:"selected"`
	Modes     []Mode  `json:"modes"` // all modes registered with macOS
	MaxPixelW int     `json:"maxPixelW"`
	MaxPixelH int     `json:"maxPixelH"`
	RefreshHz float64 `json:"refreshHz"`
	WidthMM   float64 `json:"widthMM"`
	HeightMM  float64 `json:"heightMM"`
}
