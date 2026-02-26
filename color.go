package main

import (
	"fmt"
	"os"
	"strings"
)

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
