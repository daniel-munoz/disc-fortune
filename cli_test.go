package main

import (
	"testing"
)

func TestParseInterspersedFlagsAfterPositional(t *testing.T) {
	fs := newFlagSet("favorite")
	year := fs.String("year", "", "")

	rest, err := parseInterspersed(fs, []string{"miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *year != "1959" {
		t.Errorf("year = %q, want 1959 (flag after positional was dropped)", *year)
	}
}

func TestParseInterspersedFlagsBeforePositional(t *testing.T) {
	fs := newFlagSet("favorite")
	year := fs.String("year", "", "")

	rest, err := parseInterspersed(fs, []string{"--year", "1959", "miles"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *year != "1959" {
		t.Errorf("year = %q, want 1959", *year)
	}
}

func TestParseInterspersedFlagsSurroundingPositional(t *testing.T) {
	fs := newFlagSet("favorite")
	year := fs.String("year", "", "")
	genre := fs.String("genre", "", "")

	rest, err := parseInterspersed(fs, []string{"--genre", "jazz", "miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *genre != "jazz" || *year != "1959" {
		t.Errorf("genre = %q, year = %q, want jazz/1959", *genre, *year)
	}
}

func TestParseInterspersedMultiplePositionals(t *testing.T) {
	fs := newFlagSet("favorite")
	rest, err := parseInterspersed(fs, []string{"kind", "of", "blue"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 3 {
		t.Errorf("positional = %v, want 3 items", rest)
	}
}

func TestParseInterspersedDashTerminator(t *testing.T) {
	fs := newFlagSet("favorite")
	rest, err := parseInterspersed(fs, []string{"--", "-live-"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "-live-" {
		t.Errorf("positional = %v, want [-live-]", rest)
	}
}

func TestParseInterspersedUnknownFlag(t *testing.T) {
	fs := newFlagSet("pick")
	if _, err := parseInterspersed(fs, []string{"--nope"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestFilterFlagsBuildsFilter(t *testing.T) {
	fs := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "1970-1980", "--genre", "jazz"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if filter.Year != "1970-1980" || filter.Genre != "jazz" {
		t.Errorf("filter = %+v, want Year=1970-1980 Genre=jazz", filter)
	}
	if !ff.any() {
		t.Error("any() = false, want true")
	}
}

func TestFilterFlagsRejectsBadYear(t *testing.T) {
	fs := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "nineteen"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if _, err := ff.Filter(); err == nil {
		t.Fatal("expected error for non-numeric year")
	}
}

func TestFilterFlagsAnyFalseWhenUnset(t *testing.T) {
	fs := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, nil); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if ff.any() {
		t.Error("any() = true, want false when no filter flags set")
	}
}
