package term

import (
	"os"
	"strings"
	"testing"
)

func TestIsTTY(t *testing.T) {
	// Just verify function exists and returns bool
	_ = IsTTY(os.Stdout)
}

func noEnv(string) string { return "" }

func envWith(k, v string) func(string) string {
	return func(name string) string {
		if name == k {
			return v
		}
		return ""
	}
}

func TestParseColorModeAcceptsTheThreeValues(t *testing.T) {
	for input, want := range map[string]Mode{
		"auto":   Auto,
		"always": Always,
		"never":  Never,
	} {
		got, err := ParseMode(input)
		if err != nil {
			t.Errorf("ParseMode(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseMode(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseColorModeRejectsAnythingElse(t *testing.T) {
	got, err := ParseMode("sometimes")
	if err == nil {
		t.Fatalf("ParseMode(\"sometimes\") = %v, want an error", got)
	}
	for _, want := range []string{"auto", "always", "never"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should list the valid value %q", err, want)
		}
	}
}

func TestAutoColorFollowsTheTerminal(t *testing.T) {
	if Use(Auto, false, noEnv) {
		t.Error("auto must not colorize a pipe")
	}
	if !Use(Auto, true, noEnv) {
		t.Error("auto must colorize a terminal")
	}
}

// The point of --color=always: `disc-fortune list | less -R` keeps its color.
func TestAlwaysColorSurvivesAPipe(t *testing.T) {
	if !Use(Always, false, noEnv) {
		t.Error("--color=always must colorize even when stdout is not a terminal")
	}
}

func TestNeverColorSuppressesOnATerminal(t *testing.T) {
	if Use(Never, true, noEnv) {
		t.Error("--color=never must suppress color on a terminal")
	}
}

func TestNoColorEnvDisablesColor(t *testing.T) {
	if Use(Auto, true, envWith("NO_COLOR", "1")) {
		t.Error("NO_COLOR must disable color under auto")
	}
}

// no-color.org: the variable disables color when present and *non-empty*.
func TestEmptyNoColorIsIgnored(t *testing.T) {
	if !Use(Auto, true, envWith("NO_COLOR", "")) {
		t.Error("an empty NO_COLOR must not disable color")
	}
}

// no-color.org asks that an explicit user override win over the environment.
// --color=always is exactly that override.
func TestExplicitAlwaysOverridesNoColor(t *testing.T) {
	if !Use(Always, false, envWith("NO_COLOR", "1")) {
		t.Error("--color=always must override NO_COLOR")
	}
}
