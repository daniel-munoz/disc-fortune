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
