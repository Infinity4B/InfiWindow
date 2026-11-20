//go:build !darwin

package display

import "github.com/Infinity4B/infiwindow/internal/remote/device"

// Manager is a no-op outside macOS.
type Manager struct{}

func NewManager() *Manager                             { return &Manager{} }
func (m *Manager) Create(Spec, Layout) (uint32, error) { return 0, ErrUnsupported }
func (m *Manager) Destroy()                            {}
func (m *Manager) Close()                              {}
func (m *Manager) TrackModes(bool)                     {}
func (m *Manager) AdoptModes()                         {}
func (m *Manager) Active() (uint32, bool)              { return 0, false }
func (m *Manager) Supported() bool                     { return false }
func ListDisplays() []Display                          { return nil }
func (m *Manager) ListDisplays() []Display             { return nil }
func SetMode(uint32, int, int, bool) error             { return ErrUnsupported }
