package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDiscogsReleaseURL(t *testing.T) {
	if got, want := discogsReleaseURL(1839278), "https://www.discogs.com/release/1839278"; got != want {
		t.Errorf("discogsReleaseURL = %q, want %q", got, want)
	}
}

func TestBrowserCommandPerPlatform(t *testing.T) {
	tests := []struct {
		goos string
		name string
		args []string
		ok   bool
	}{
		{"darwin", "open", nil, true},
		{"linux", "xdg-open", nil, true},
		{"freebsd", "xdg-open", nil, true},
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler"}, true},
		{"plan9", "", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			name, args, ok := browserCommand(tc.goos)
			if ok != tc.ok || name != tc.name {
				t.Fatalf("browserCommand(%q) = %q, %v, %v; want %q, %v, %v",
					tc.goos, name, args, ok, tc.name, tc.args, tc.ok)
			}
			if strings.Join(args, " ") != strings.Join(tc.args, " ") {
				t.Errorf("args = %v, want %v", args, tc.args)
			}
		})
	}
}

// darwin and windows have no DISPLAY and must not be judged by it.
func TestNeedsDisplay(t *testing.T) {
	for goos, want := range map[string]bool{"darwin": false, "windows": false, "linux": true, "freebsd": true} {
		if got := needsDisplay(goos); got != want {
			t.Errorf("needsDisplay(%q) = %v, want %v", goos, got, want)
		}
	}
}

// Stand-ins for exec.LookPath and os.Getenv, so the policy can be exercised
// with no browser, no display and no PATH.
func lookPathFound(string) (string, error)   { return "/usr/bin/whatever", nil }
func lookPathMissing(string) (string, error) { return "", errors.New("not found") }

func envWithDisplay(k string) string {
	if k == "DISPLAY" {
		return ":0"
	}
	return ""
}
func envNoDisplay(string) string { return "" }

const openTestURL = "https://www.discogs.com/release/1"

func TestPlanOpenLaunches(t *testing.T) {
	plan := planOpen(openTestURL, false, "linux", lookPathFound, envWithDisplay)
	want := []string{"xdg-open", openTestURL}
	if strings.Join(plan.Launch, " ") != strings.Join(want, " ") {
		t.Errorf("Launch = %v, want %v", plan.Launch, want)
	}
	if plan.Note != "" {
		t.Errorf("Note = %q, want empty on a successful launch", plan.Note)
	}
}

func TestPlanOpenWindowsPutsTheURLLast(t *testing.T) {
	plan := planOpen(openTestURL, false, "windows", lookPathFound, envNoDisplay)
	want := []string{"rundll32", "url.dll,FileProtocolHandler", openTestURL}
	if strings.Join(plan.Launch, " ") != strings.Join(want, " ") {
		t.Errorf("Launch = %v, want %v", plan.Launch, want)
	}
}

// --print never consults the launcher or the environment, so it behaves
// identically everywhere.
func TestPlanOpenPrintOnlyIsSilent(t *testing.T) {
	plan := planOpen(openTestURL, true, "linux", lookPathFound, envWithDisplay)
	if plan.Launch != nil {
		t.Errorf("Launch = %v, want nil under --print", plan.Launch)
	}
	if plan.Note != "" {
		t.Errorf("Note = %q, want empty under --print", plan.Note)
	}
}

func TestPlanOpenFallsBackWhenLauncherIsMissing(t *testing.T) {
	plan := planOpen(openTestURL, false, "linux", lookPathMissing, envWithDisplay)
	if plan.Launch != nil {
		t.Errorf("Launch = %v, want nil when the launcher is not on PATH", plan.Launch)
	}
	if !strings.Contains(plan.Note, "xdg-open") {
		t.Errorf("Note = %q, want it to name the missing launcher", plan.Note)
	}
}

func TestPlanOpenFallsBackWithNoDisplay(t *testing.T) {
	plan := planOpen(openTestURL, false, "linux", lookPathFound, envNoDisplay)
	if plan.Launch != nil {
		t.Errorf("Launch = %v, want nil with no display", plan.Launch)
	}
	if !strings.Contains(plan.Note, "display") {
		t.Errorf("Note = %q, want it to mention the display", plan.Note)
	}
}

func TestPlanOpenAcceptsWaylandAlone(t *testing.T) {
	wayland := func(k string) string {
		if k == "WAYLAND_DISPLAY" {
			return "wayland-0"
		}
		return ""
	}
	if plan := planOpen(openTestURL, false, "linux", lookPathFound, wayland); plan.Launch == nil {
		t.Errorf("Launch = nil under Wayland with no X11 DISPLAY; note = %q", plan.Note)
	}
}

// darwin has no DISPLAY, so an empty environment must not stop a launch.
func TestPlanOpenDarwinIgnoresDisplay(t *testing.T) {
	if plan := planOpen(openTestURL, false, "darwin", lookPathFound, envNoDisplay); plan.Launch == nil {
		t.Errorf("Launch = nil on darwin with no DISPLAY; note = %q", plan.Note)
	}
}

func TestPlanOpenUnknownPlatformFallsBack(t *testing.T) {
	plan := planOpen(openTestURL, false, "plan9", lookPathFound, envNoDisplay)
	if plan.Launch != nil {
		t.Errorf("Launch = %v, want nil on an unknown platform", plan.Launch)
	}
	if !strings.Contains(plan.Note, "plan9") {
		t.Errorf("Note = %q, want it to name the platform", plan.Note)
	}
}

// launchBrowser must not wait: a launcher that blocks would hang the CLI.
// `true` exits immediately, so this only proves Start succeeds and returns.
func TestLaunchBrowserStartsWithoutWaiting(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("no `true` on PATH")
	}
	if err := launchBrowser([]string{"true"}); err != nil {
		t.Errorf("launchBrowser: %v", err)
	}
}

func TestLaunchBrowserReportsAStartFailure(t *testing.T) {
	if err := launchBrowser([]string{"disc-fortune-no-such-binary-xyzzy"}); err == nil {
		t.Error("launchBrowser returned nil for a binary that does not exist")
	}
}
