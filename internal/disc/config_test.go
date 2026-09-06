package disc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// seedCollectionAt writes a one-album collection.json into dir. The root
// package's env_conventions_test.go keeps its own copy: the two packages
// cannot share an unexported test helper.
func seedCollectionAt(t *testing.T, dir string, album Album) {
	t.Helper()
	if err := os.MkdirAll(dir, configDirPerms); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	data, err := json.Marshal([]Album{album})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "collection.json"), data, collectionFilePerms); err != nil {
		t.Fatalf("seeding collection: %v", err)
	}
}

// envMap turns a map into the getenv function ResolveDir expects.
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

	loc, err := ResolveDir(envMap(nil), homeAt(home))
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
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
	seedCollectionAt(t, xdgDir, Album{Artist: "Sun Ra", Title: "Lanquidity"})

	loc, err := ResolveDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), homeAt(home))
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
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
	seedCollectionAt(t, legacy, Album{Artist: "Miles Davis", Title: "Kind of Blue"})

	loc, err := ResolveDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), homeAt(home))
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
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

	loc, err := ResolveDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), homeAt(home))
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
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

	loc, err := ResolveDir(envMap(map[string]string{"XDG_CONFIG_HOME": "relative/path"}), homeAt(home))
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	if want := filepath.Join(home, ".config", "disc-fortune"); loc.Dir != want {
		t.Errorf("Dir = %q, want %q", loc.Dir, want)
	}
}

func TestConfigDirTreatsEmptyXDGAsUnset(t *testing.T) {
	home := t.TempDir()

	loc, err := ResolveDir(envMap(map[string]string{"XDG_CONFIG_HOME": ""}), homeAt(home))
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	if want := filepath.Join(home, ".config", "disc-fortune"); loc.Dir != want {
		t.Errorf("Dir = %q, want %q", loc.Dir, want)
	}
}

// Previously this called os.Exit(1), which made it untestable.
func TestConfigDirReturnsErrorWhenHomeIsUnknown(t *testing.T) {
	_, err := ResolveDir(envMap(nil), noHome)
	if err == nil {
		t.Fatal("ResolveDir must return an error when the home directory is unknown")
	}
}

func TestConfigDirSurvivesUnknownHomeWhenXDGIsSet(t *testing.T) {
	xdg := t.TempDir()

	loc, err := ResolveDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), noHome)
	if err != nil {
		t.Fatalf("XDG_CONFIG_HOME alone is enough to locate config: %v", err)
	}
	if want := filepath.Join(xdg, "disc-fortune"); loc.Dir != want {
		t.Errorf("Dir = %q, want %q", loc.Dir, want)
	}
}

// An empty directory is not a collection. Anything can create
// $XDG_CONFIG_HOME/disc-fortune -- a dotfile manager, a package, a user
// running mkdir, or a migrate that failed part-way -- and if mere existence
// were enough to win, the user's real collection would silently disappear
// behind it with no way back. This is the exact failure ResolveDir
// exists to prevent, so it must be decided on data, not on existence.
func TestConfigDirIgnoresAnEmptyXDGDirectory(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	legacy := filepath.Join(home, ".config", "disc-fortune")
	seedCollectionAt(t, legacy, Album{Artist: "Alice Coltrane", Title: "Ptah, the El Daoud"})
	if err := os.MkdirAll(filepath.Join(xdg, "disc-fortune"), configDirPerms); err != nil {
		t.Fatalf("seeding empty xdg dir: %v", err)
	}

	loc, err := ResolveDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), homeAt(home))
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	if loc.Dir != legacy {
		t.Fatalf("Dir = %q, want the legacy %q -- an empty directory shadowed the real collection", loc.Dir, legacy)
	}
	if want := filepath.Join(xdg, "disc-fortune"); loc.Preferred != want {
		t.Errorf("Preferred = %q, want %q -- without it, `migrate` reports nothing to do", loc.Preferred, want)
	}
}

// A directory holding any one of the data files counts as in use.
func TestConfigDirTreatsAnyDataFileAsInUse(t *testing.T) {
	for _, name := range []string{"collection.json", "favorites.json", "history.json", "meta.json"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			xdg := t.TempDir()
			xdgDir := filepath.Join(xdg, "disc-fortune")
			if err := os.MkdirAll(xdgDir, configDirPerms); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(xdgDir, name), []byte("[]"), collectionFilePerms); err != nil {
				t.Fatalf("seeding: %v", err)
			}
			seedCollectionAt(t, filepath.Join(home, ".config", "disc-fortune"), Album{Artist: "A", Title: "B"})

			loc, err := ResolveDir(envMap(map[string]string{"XDG_CONFIG_HOME": xdg}), homeAt(home))
			if err != nil {
				t.Fatalf("ResolveDir: %v", err)
			}
			if loc.Dir != xdgDir {
				t.Errorf("Dir = %q, want %q -- %s should mark the directory as in use", loc.Dir, xdgDir, name)
			}
		})
	}
}
