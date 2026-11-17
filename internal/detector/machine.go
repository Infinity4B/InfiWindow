// Package detector notices when a remote-desktop session (UU 远程 by
// default) is streaming this Mac's screen.
//
// UU Remote has no public API or session event, so the detector watches what
// a session cannot hide: while someone is viewing the screen, the UURemote
// processes upload a video stream. A sustained upload rate above
// Params.ConnectKBps means a session started; when the upload stays below
// Params.HoldKBps (and no extra peer connections remain open) for
// Params.DisconnectSeconds, it ended. Upload, not download, is measured, so
// using this Mac to control another device does not count as a session.
package detector

import "time"

// Params tune the detector. All rates are in KB/s of upload.
type Params struct {
	ConnectKBps       float64 `json:"connectKBps"`
	ConnectSeconds    int     `json:"connectSeconds"`
	HoldKBps          float64 `json:"holdKBps"`
	DisconnectSeconds int     `json:"disconnectSeconds"`
}

// DefaultParams suit UU Remote streaming a desktop: even a still screen keeps
// a few KB/s flowing, and a session start pushes hundreds of KB/s.
func DefaultParams() Params {
	return Params{ConnectKBps: 120, ConnectSeconds: 2, HoldKBps: 6, DisconnectSeconds: 25}
}

// Normalize clamps p into sane ranges.
func (p Params) Normalize() Params {
	d := DefaultParams()
	if p.ConnectKBps <= 0 {
		p.ConnectKBps = d.ConnectKBps
	}
	if p.ConnectSeconds < 1 {
		p.ConnectSeconds = d.ConnectSeconds
	}
	if p.HoldKBps < 0 {
		p.HoldKBps = 0
	}
	if p.HoldKBps > p.ConnectKBps {
		p.HoldKBps = p.ConnectKBps
	}
	if p.DisconnectSeconds < 3 {
		p.DisconnectSeconds = 3
	}
	return p
}

// Machine turns samples into a connected/disconnected state.
type Machine struct {
	p          Params
	connected  bool
	aboveSince time.Time
	lastActive time.Time
	// baseline is the number of peer connections seen while idle; more
	// than that during a session counts as activity. -1 = unknown.
	baseline int
}

// NewMachine returns a disconnected machine.
func NewMachine(p Params) *Machine {
	return &Machine{p: p.Normalize(), baseline: -1}
}

// SetParams changes the thresholds without losing the current state.
func (m *Machine) SetParams(p Params) { m.p = p.Normalize() }

// Connected reports the current state.
func (m *Machine) Connected() bool { return m.connected }

// Step feeds one sample: the upload rate and the number of peer connections
// (-1 when unknown). It returns true when the state changed.
func (m *Machine) Step(now time.Time, uploadKBps float64, conns int) bool {
	if !m.connected {
		if uploadKBps >= m.p.ConnectKBps {
			if m.aboveSince.IsZero() {
				m.aboveSince = now
			}
			// A sample covers the second before it, so N samples above the
			// threshold span N seconds.
			if now.Sub(m.aboveSince) >= time.Duration(m.p.ConnectSeconds-1)*time.Second {
				m.connected = true
				m.lastActive = now
				m.aboveSince = time.Time{}
				return true
			}
			return false
		}
		m.aboveSince = time.Time{}
		if conns >= 0 {
			m.baseline = conns
		}
		return false
	}

	active := uploadKBps >= m.p.HoldKBps
	if conns >= 0 && m.baseline >= 0 && conns > m.baseline {
		active = true
	}
	if active {
		m.lastActive = now
		return false
	}
	if now.Sub(m.lastActive) >= time.Duration(m.p.DisconnectSeconds)*time.Second {
		m.connected = false
		return true
	}
	return false
}

// Force sets the state, e.g. when the user overrides detection.
func (m *Machine) Force(connected bool, now time.Time) {
	m.connected = connected
	m.lastActive = now
	m.aboveSince = time.Time{}
}
