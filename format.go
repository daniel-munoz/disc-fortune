package main

import (
	"fmt"
	"strings"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

// formatAlbum formats an album for display with optional color.
func formatAlbum(album disc.Album, useColor bool) string {
	var sb strings.Builder

	// First line: Artist - Title
	if useColor {
		sb.WriteString(term.BoldCyan)
		sb.WriteString(album.Artist)
		sb.WriteString(term.Reset)
		sb.WriteString(" - ")
		sb.WriteString(term.BoldWhite)
		sb.WriteString(album.Title)
		sb.WriteString(term.Reset)
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
			sb.WriteString(term.Dim)
		}
		sb.WriteString(strings.Join(metadata, " · "))
		if useColor {
			sb.WriteString(term.Reset)
		}
	}

	return sb.String()
}
