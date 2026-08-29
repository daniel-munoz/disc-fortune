package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// appName is the per-application directory disc-fortune owns inside whichever
// config root is in effect.
const appName = "disc-fortune"

// configLocation says where disc-fortune's data lives, and where it ought to
// live if those differ.
type configLocation struct {
	// Dir is the directory actually used for reads and writes.
	Dir string
	// Preferred is the XDG-derived directory that Dir is standing in for,
	// or "" when Dir is already the right place. A non-empty Preferred is
	// what makes `disc-fortune migrate` meaningful.
	Preferred string
}

// resolveConfigDir decides where the data files live, honoring
// XDG_CONFIG_HOME. getenv and homeDir are injected so the decision can be
// tested without touching the real environment.
//
// The awkward case is an existing user who has had XDG_CONFIG_HOME set all
// along: their data is in the legacy ~/.config/disc-fortune, and naively
// switching to the XDG path on upgrade would make their entire collection
// appear to vanish. So an XDG directory that does not yet exist never
// displaces legacy data that does — the legacy path keeps being used, and
// Preferred records where a migration would put it.
func resolveConfigDir(getenv func(string) string, homeDir func() (string, error)) (configLocation, error) {
	xdg := getenv("XDG_CONFIG_HOME")
	// The XDG basedir spec says a relative path is invalid and must be
	// ignored, falling back to the default.
	if !filepath.IsAbs(xdg) {
		xdg = ""
	}

	home, homeErr := homeDir()

	var legacy string
	if homeErr == nil {
		legacy = filepath.Join(home, ".config", appName)
	}

	if xdg == "" {
		if homeErr != nil {
			return configLocation{}, fmt.Errorf("cannot determine home directory: %w", homeErr)
		}
		return configLocation{Dir: legacy}, nil
	}

	xdgDir := filepath.Join(xdg, appName)
	if isDir(xdgDir) {
		return configLocation{Dir: xdgDir}, nil
	}
	if legacy != "" && isDir(legacy) {
		return configLocation{Dir: legacy, Preferred: xdgDir}, nil
	}
	// Nothing exists yet: a fresh install goes straight to the right place.
	return configLocation{Dir: xdgDir}, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
