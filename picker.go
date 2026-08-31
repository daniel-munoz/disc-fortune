package main

import "fmt"

// drawMode selects how pick draws from the candidate pool.
type drawMode int

const (
	// drawFresh excludes the recently played. It is the zero value so a
	// selection built without an explicit mode gets the default rather than
	// silently falling back to an unfiltered draw.
	drawFresh drawMode = iota
	// drawAny is a uniform draw; history is not consulted at all. This is
	// what restores pre-2.3 behavior for anyone scripting against it.
	drawAny
	// drawStale is drawFresh's exclusion followed by a bias toward the
	// records left unplayed longest.
	drawStale
)

// parseDrawMode converts the --draw flag value to a drawMode.
func parseDrawMode(s string) (drawMode, error) {
	switch s {
	case "fresh":
		return drawFresh, nil
	case "any":
		return drawAny, nil
	case "stale":
		return drawStale, nil
	default:
		return drawFresh, fmt.Errorf("invalid --draw value %q (want any, fresh, or stale)", s)
	}
}
