package main

import (
	"os"
	"strings"
	"testing"
)

func TestFormatAlbum(t *testing.T) {
	album := Album{
		Artist:  "Miles Davis",
		Title:   "Kind of Blue",
		Year:    1959,
		Label:   "Columbia",
		CatNo:   "CL 1355",
		Genres:  []string{"Jazz"},
		Formats: []string{"Vinyl", "12\""},
	}

	// Test with color
	output := formatAlbum(album, true)
	if !strings.Contains(output, "Miles Davis") {
		t.Error("output missing artist")
	}
	if !strings.Contains(output, "Kind of Blue") {
		t.Error("output missing title")
	}
	if !strings.Contains(output, "1959") {
		t.Error("output missing year")
	}
	if !strings.Contains(output, "\033[") {
		t.Error("output missing ANSI codes")
	}

	// Test without color
	output = formatAlbum(album, false)
	if strings.Contains(output, "\033[") {
		t.Error("output should not have ANSI codes")
	}
}

func TestIsTTY(t *testing.T) {
	// Just verify function exists and returns bool
	_ = isTTY(os.Stdout)
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
	for input, want := range map[string]colorMode{
		"auto":   colorAuto,
		"always": colorAlways,
		"never":  colorNever,
	} {
		got, err := parseColorMode(input)
		if err != nil {
			t.Errorf("parseColorMode(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("parseColorMode(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseColorModeRejectsAnythingElse(t *testing.T) {
	got, err := parseColorMode("sometimes")
	if err == nil {
		t.Fatalf("parseColorMode(\"sometimes\") = %v, want an error", got)
	}
	for _, want := range []string{"auto", "always", "never"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should list the valid value %q", err, want)
		}
	}
}

func TestAutoColorFollowsTheTerminal(t *testing.T) {
	if useColor(colorAuto, false, noEnv) {
		t.Error("auto must not colorize a pipe")
	}
	if !useColor(colorAuto, true, noEnv) {
		t.Error("auto must colorize a terminal")
	}
}

// The point of --color=always: `disc-fortune list | less -R` keeps its color.
func TestAlwaysColorSurvivesAPipe(t *testing.T) {
	if !useColor(colorAlways, false, noEnv) {
		t.Error("--color=always must colorize even when stdout is not a terminal")
	}
}

func TestNeverColorSuppressesOnATerminal(t *testing.T) {
	if useColor(colorNever, true, noEnv) {
		t.Error("--color=never must suppress color on a terminal")
	}
}

func TestNoColorEnvDisablesColor(t *testing.T) {
	if useColor(colorAuto, true, envWith("NO_COLOR", "1")) {
		t.Error("NO_COLOR must disable color under auto")
	}
}

// no-color.org: the variable disables color when present and *non-empty*.
func TestEmptyNoColorIsIgnored(t *testing.T) {
	if !useColor(colorAuto, true, envWith("NO_COLOR", "")) {
		t.Error("an empty NO_COLOR must not disable color")
	}
}

// no-color.org asks that an explicit user override win over the environment.
// --color=always is exactly that override.
func TestExplicitAlwaysOverridesNoColor(t *testing.T) {
	if !useColor(colorAlways, false, envWith("NO_COLOR", "1")) {
		t.Error("--color=always must override NO_COLOR")
	}
}
