//go:build !darwin

package vdisplay

import "github.com/Infinity4B/infiwindow/internal/device"

// Manager is a no-op outside macOS.
type Manager struct{}

func NewManager() *Manager                                    { return &Manager{} }
func (m *Manager) Create(device.Spec, Layout) (uint32, error) { return 0, ErrUnsupported }
func (m *Manager) Destroy()                                   {}
func (m *Manager) Active() (uint32, bool)                     { return 0, false }
func (m *Manager) Supported() bool                            { return false }
func ListDisplays() []Display                                 { return nil }
func (m *Manager) ListDisplays() []Display                    { return nil }
