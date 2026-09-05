package main

import (
	"fmt"
	"os/exec"
)

// discogsReleaseURL is where a release lives on Discogs.
func discogsReleaseURL(releaseID int) string {
	return fmt.Sprintf("https://www.discogs.com/release/%d", releaseID)
}

// browserCommand returns the argv prefix that opens a URL on goos, and
// whether goos is one we know how to do that on. The caller appends the URL.
//
// goos is a parameter rather than a read of runtime.GOOS so that every branch
// is testable from any host; a runtime.GOOS switch would only ever exercise
// one of them in CI.
func browserCommand(goos string) (name string, args []string, ok bool) {
	switch goos {
	case "darwin":
		return "open", nil, true
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		return "xdg-open", nil, true
	case "windows":
		// rundll32 rather than `cmd /c start`, which treats & in a URL as a
		// command separator.
		return "rundll32", []string{"url.dll,FileProtocolHandler"}, true
	}
	return "", nil, false
}

// needsDisplay reports whether goos routes browser launching through a
// display server, so an unset DISPLAY means there is nothing to launch into.
// darwin and windows have no DISPLAY and must not be judged by it.
func needsDisplay(goos string) bool {
	switch goos {
	case "darwin", "windows":
		return false
	}
	return true
}

// openPlan is what runOpen should do.
type openPlan struct {
	// Launch is the argv to start, or nil when the URL should be printed
	// instead.
	Launch []string
	// Note explains why nothing is being launched. It is empty under
	// --print, which is a deliberate choice rather than a degradation.
	Note string
}

// planOpen decides between launching and printing without doing either.
// Everything it consults arrives as a parameter, so the whole policy is
// testable with no browser, no display and no PATH.
//
// Printing is a graceful degradation rather than a failure: a script on a
// headless box gets a usable URL on the data channel, which is what it
// wanted. The caller therefore exits 0 on every path here.
func planOpen(url string, printOnly bool, goos string,
	lookPath func(string) (string, error), getenv func(string) string) openPlan {

	if printOnly {
		return openPlan{}
	}

	name, args, ok := browserCommand(goos)
	if !ok {
		return openPlan{Note: fmt.Sprintf(
			"disc-fortune: no browser launcher known for %s; printed the URL instead.", goos)}
	}
	if needsDisplay(goos) && getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
		return openPlan{Note: "disc-fortune: no display found; printed the URL instead."}
	}
	if _, err := lookPath(name); err != nil {
		return openPlan{Note: fmt.Sprintf(
			"disc-fortune: %s is not on PATH; printed the URL instead.", name)}
	}

	return openPlan{Launch: append(append([]string{name}, args...), url)}
}

// launchBrowser starts argv and does not wait for it.
//
// Start rather than Run: xdg-open blocks until the browser exits on some
// desktops, and hanging the user's terminal is a worse failure than missing a
// launcher's exit code. The consequence, accepted: a launcher that starts and
// then fails is invisible to us, which is tolerable because the browser
// window is the user's own feedback.
func launchBrowser(argv []string) error {
	return exec.Command(argv[0], argv[1:]...).Start()
}
