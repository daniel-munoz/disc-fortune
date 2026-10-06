package cli

import "testing"

func TestParseInterspersedFlagsAfterPositional(t *testing.T) {
	fs, _ := NewFlagSet("favorite")
	year := fs.String("year", "", "")

	rest, err := ParseInterspersed(fs, []string{"miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("ParseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *year != "1959" {
		t.Errorf("year = %q, want 1959 (flag after positional was dropped)", *year)
	}
}

func TestParseInterspersedFlagsBeforePositional(t *testing.T) {
	fs, _ := NewFlagSet("favorite")
	year := fs.String("year", "", "")

	rest, err := ParseInterspersed(fs, []string{"--year", "1959", "miles"})
	if err != nil {
		t.Fatalf("ParseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *year != "1959" {
		t.Errorf("year = %q, want 1959", *year)
	}
}

func TestParseInterspersedFlagsSurroundingPositional(t *testing.T) {
	fs, _ := NewFlagSet("favorite")
	year := fs.String("year", "", "")
	genre := fs.String("genre", "", "")

	rest, err := ParseInterspersed(fs, []string{"--genre", "jazz", "miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("ParseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *genre != "jazz" || *year != "1959" {
		t.Errorf("genre = %q, year = %q, want jazz/1959", *genre, *year)
	}
}

func TestParseInterspersedMultiplePositionals(t *testing.T) {
	fs, _ := NewFlagSet("favorite")
	rest, err := ParseInterspersed(fs, []string{"kind", "of", "blue"})
	if err != nil {
		t.Fatalf("ParseInterspersed: %v", err)
	}
	if len(rest) != 3 {
		t.Errorf("positional = %v, want 3 items", rest)
	}
}

func TestParseInterspersedDashTerminator(t *testing.T) {
	fs, _ := NewFlagSet("favorite")
	rest, err := ParseInterspersed(fs, []string{"--", "-live-"})
	if err != nil {
		t.Fatalf("ParseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "-live-" {
		t.Errorf("positional = %v, want [-live-]", rest)
	}
}

func TestParseInterspersedUnknownFlag(t *testing.T) {
	fs, _ := NewFlagSet("pick")
	if _, err := ParseInterspersed(fs, []string{"--nope"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}
