// Package config stores InfiWindow's settings as JSON, one section per
// feature.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/Infinity4B/infiwindow/internal/remote"
)

// Config is everything the user can change. External-monitor HiDPI keeps
// its state in the display override files themselves, so it has no section.
type Config struct {
	Remote remote.Config `json:"remote"`
}

// Default is the configuration of a fresh install.
func Default() Config {
	return Config{Remote: remote.DefaultConfig()}
}

// Normalize fills in missing or invalid values with defaults.
func (c Config) Normalize() Config {
	c.Remote = c.Remote.Normalize()
	return c
}

// Load reads path, returning defaults when it does not exist. Files from
// before the per-feature sections (the remote settings at the top level)
// are read too.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil {
		return Default(), err
	}
	c := Default()
	// Clear profile fields before unmarshaling so Normalize can tell whether an
	// older file only populated the legacy "device" field without "profiles".
	c.Remote.Active = ""
	c.Remote.Profiles = remote.Profiles{}
	if _, ok := probe["remote"]; !ok {
		if _, legacy := probe["device"]; legacy {
			err := json.Unmarshal(b, &c.Remote)
			return c.Normalize(), err
		}
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), err
	}
	return c.Normalize(), nil
}

// Save writes c to path atomically.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
