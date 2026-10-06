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

// formatMatch formats one candidate of an ambiguous query: the album, plus
// its release ID on a dim line of its own. Two pressings of a title can be
// identical in artist, title, year, label, catalogue number and genre -- two
// store-exclusive colours, say -- and then the ID is the only thing that
// tells them apart, as well as the only thing --release-id can act on.
func formatMatch(album disc.Album, useColor bool) string {
	out := formatAlbum(album, useColor)
	if album.ReleaseID == 0 {
		return out
	}
	line := fmt.Sprintf("release %d", album.ReleaseID)
	if useColor {
		line = term.Dim + line + term.Reset
	}
	return out + "\n" + line
}

// formatList formats a slice of albums for list display.
// Albums are separated by blank lines; a count summary is appended.
//
// showIDs is set only where the user has to choose between candidates. Plain
// `list` leaves it off, so everyday output is unchanged.
func formatList(albums []disc.Album, useColor, showIDs bool) string {
	if len(albums) == 0 {
		return "No albums match the specified filters\n"
	}
	var sb strings.Builder
	for i, album := range albums {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if showIDs {
			sb.WriteString(formatMatch(album, useColor))
		} else {
			sb.WriteString(formatAlbum(album, useColor))
		}
	}
	noun := "albums"
	if len(albums) == 1 {
		noun = "album"
	}
	sb.WriteString(fmt.Sprintf("\n\n%d %s\n", len(albums), noun))
	return sb.String()
}
