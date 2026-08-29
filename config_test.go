package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// envMap turns a map into the getenv function resolveConfigDir expects.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func homeAt(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

func noHome() (string, error) {
	return "", errors.New("$HOME is not defined")
}

func TestConfigDirDefaultsToLegacyPathWithoutXDG(t *testing.T) {
	home := t.TempDir()

	loc, err := resolveConfigDir(envMap(nil), homeAt(home))
	if err != nil {
		t.Fatalf("resolveConfigDir: %v", err)
	}
	want := filepath.Join(home, ".config", "disc-fortune")
	if loc.Dir != want {
		t.Errorf("Dir = %q, want %q", loc.Dir, want)
	}
	if loc.Preferred != "" {
		t.Errorf("Preferred = %q, want empty when XDG_CONFIG_HOME is unset", loc.Preferred)
	}
}

func TestConfigDirUsesXDGWhenItAlreadyHasData(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	xdgDir := filepath.Join(xdg, "disc-fortune")
	if err := os.MkdirAll(xdgDir, configDirPerms); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	loc, err := resolveConfigDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), homeAt(home))
	if err != nil {
		t.Fatalf("resolveConfigDir: %v", err)
	}
	if loc.Dir != xdgDir {
		t.Errorf("Dir = %q, want %q", loc.Dir, xdgDir)
	}
	if loc.Preferred != "" {
		t.Errorf("Preferred = %q, want empty when XDG is already in use", loc.Preferred)
	}
}

// The upgrade hazard the roadmap warns about: a user who has had
// XDG_CONFIG_HOME set all along, with their real data in the legacy path.
// Naively honoring XDG would make their whole collection appear to vanish.
func TestConfigDirKeepsLegacyDataWhenXDGIsEmpty(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	legacy := filepath.Join(home, ".config", "disc-fortune")
	if err := os.MkdirAll(legacy, configDirPerms); err != nil {
		t.Fatalf("seeding legacy: %v", err)
	}

	loc, err := resolveConfigDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), homeAt(home))
	if err != nil {
		t.Fatalf("resolveConfigDir: %v", err)
	}
	if loc.Dir != legacy {
		t.Fatalf("Dir = %q, want the legacy %q -- honoring XDG here loses the user's collection", loc.Dir, legacy)
	}
	if want := filepath.Join(xdg, "disc-fortune"); loc.Preferred != want {
		t.Errorf("Preferred = %q, want %q so the user can be told where to migrate", loc.Preferred, want)
	}
}

func TestConfigDirPrefersXDGForAFreshInstall(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()

	loc, err := resolveConfigDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), homeAt(home))
	if err != nil {
		t.Fatalf("resolveConfigDir: %v", err)
	}
	if want := filepath.Join(xdg, "disc-fortune"); loc.Dir != want {
		t.Errorf("Dir = %q, want %q", loc.Dir, want)
	}
	if loc.Preferred != "" {
		t.Errorf("Preferred = %q, want empty -- there is nothing to migrate", loc.Preferred)
	}
}

// The XDG spec says a relative XDG_CONFIG_HOME is invalid and must be ignored.
func TestConfigDirIgnoresRelativeXDG(t *testing.T) {
	home := t.TempDir()

	loc, err := resolveConfigDir(envMap(map[string]string{"XDG_CONFIG_HOME": "relative/path"}), homeAt(home))
	if err != nil {
		t.Fatalf("resolveConfigDir: %v", err)
	}
	if want := filepath.Join(home, ".config", "disc-fortune"); loc.Dir != want {
		t.Errorf("Dir = %q, want %q", loc.Dir, want)
	}
}

func TestConfigDirTreatsEmptyXDGAsUnset(t *testing.T) {
	home := t.TempDir()

	loc, err := resolveConfigDir(envMap(map[string]string{"XDG_CONFIG_HOME": ""}), homeAt(home))
	if err != nil {
		t.Fatalf("resolveConfigDir: %v", err)
	}
	if want := filepath.Join(home, ".config", "disc-fortune"); loc.Dir != want {
		t.Errorf("Dir = %q, want %q", loc.Dir, want)
	}
}

// Previously this called os.Exit(1), which made it untestable.
func TestConfigDirReturnsErrorWhenHomeIsUnknown(t *testing.T) {
	_, err := resolveConfigDir(envMap(nil), noHome)
	if err == nil {
		t.Fatal("resolveConfigDir must return an error when the home directory is unknown")
	}
}

func TestConfigDirSurvivesUnknownHomeWhenXDGIsSet(t *testing.T) {
	xdg := t.TempDir()

	loc, err := resolveConfigDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), noHome)
	if err != nil {
		t.Fatalf("XDG_CONFIG_HOME alone is enough to locate config: %v", err)
	}
	if want := filepath.Join(xdg, "disc-fortune"); loc.Dir != want {
		t.Errorf("Dir = %q, want %q", loc.Dir, want)
	}
}
