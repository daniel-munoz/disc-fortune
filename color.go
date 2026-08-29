package main

import (
	"fmt"
	"os"
	"strings"
)

// colorMode is the resolved value of --color.
type colorMode int

const (
	// colorAuto colorizes only when writing to a terminal and NO_COLOR is unset.
	colorAuto colorMode = iota
	colorAlways
	colorNever
)

// parseColorMode converts the --color flag value to a colorMode.
func parseColorMode(s string) (colorMode, error) {
	switch s {
	case "auto":
		return colorAuto, nil
	case "always":
		return colorAlways, nil
	case "never":
		return colorNever, nil
	default:
		return colorAuto, fmt.Errorf("invalid --color value %q (want auto, always, or never)", s)
	}
}

// useColor decides whether to emit escape sequences, given the resolved
// --color mode and whether the destination is a terminal.
//
// An explicit --color=always or --color=never always wins: no-color.org asks
// that NO_COLOR be overridable by the user's own instruction, and someone who
// typed --color=always meant it. Only under auto does NO_COLOR apply, and
// then only when non-empty, again per no-color.org.
func useColor(mode colorMode, tty bool, getenv func(string) string) bool {
	switch mode {
	case colorAlways:
		return true
	case colorNever:
		return false
	}
	if getenv("NO_COLOR") != "" {
		return false
	}
	return tty
}

const (
	colorReset     = "\033[0m"
	colorBoldCyan  = "\033[1;36m"
	colorBoldWhite = "\033[1;37m"
	colorDim       = "\033[2m"
)

// isTTY returns true if the file is a terminal.
func isTTY(f *os.File) bool {
	fileInfo, err := f.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// formatAlbum formats an album for display with optional color.
func formatAlbum(album Album, useColor bool) string {
	var sb strings.Builder

	// First line: Artist - Title
	if useColor {
		sb.WriteString(colorBoldCyan)
		sb.WriteString(album.Artist)
		sb.WriteString(colorReset)
		sb.WriteString(" - ")
		sb.WriteString(colorBoldWhite)
		sb.WriteString(album.Title)
		sb.WriteString(colorReset)
	} else {
		sb.WriteString(album.Artist)
		sb.WriteString(" - ")
		sb.WriteString(album.Title)
	}

	// Second line: metadata (if any)
	var metadata []string
	if album.Year != 0 {
		metadata = append(metadata, fmt.Sprintf("%d", album.Year))
	}
	if album.Label != "" {
		metadata = append(metadata, album.Label)
	}
	if album.CatNo != "" {
		metadata = append(metadata, album.CatNo)
	}
	if len(album.Genres) > 0 {
		metadata = append(metadata, strings.Join(album.Genres, ", "))
	}

	if len(metadata) > 0 {
		sb.WriteString("\n")
		if useColor {
			sb.WriteString(colorDim)
		}
		sb.WriteString(strings.Join(metadata, " · "))
		if useColor {
			sb.WriteString(colorReset)
		}
	}

	return sb.String()
}
