package main

import (
	"strings"
	"testing"
)

func TestPickAcceptsColorFlag(t *testing.T) {
	cfg, err := parseSelection("pick", []string{"--color", "always"})
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if cfg.color != colorAlways {
		t.Errorf("color = %v, want colorAlways", cfg.color)
	}
}

func TestColorFlagRejectsUnknownValue(t *testing.T) {
	_, err := parseSelection("pick", []string{"--color", "sometimes"})
	if err == nil {
		t.Fatal("expected a usage error for --color=sometimes")
	}
	if !strings.Contains(err.Error(), "pick") {
		t.Errorf("error %q should name the command", err)
	}
}

func TestHistoryAcceptsColorFlag(t *testing.T) {
	cfg, err := parseHistory([]string{"--color", "never", "5"})
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if cfg.color != colorNever {
		t.Errorf("color = %v, want colorNever", cfg.color)
	}
	if cfg.limit != 5 {
		t.Errorf("limit = %d, want 5", cfg.limit)
	}
}

func TestFavoriteAcceptsColorFlag(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"miles", "--color", "never"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.color != colorNever {
		t.Errorf("color = %v, want colorNever", cfg.color)
	}
}

// The point of registering global flags in newFlagSet: no command can miss
// them. This is the test that keeps a future command honest.
func TestEveryCommandAcceptsColorFlag(t *testing.T) {
	parsers := map[string]func([]string) error{
		"pick":       func(a []string) error { _, err := parseSelection("pick", a); return err },
		"list":       func(a []string) error { _, err := parseSelection("list", a); return err },
		"favorite":   func(a []string) error { _, err := parseFavorite("favorite", a); return err },
		"unfavorite": func(a []string) error { _, err := parseFavorite("unfavorite", a); return err },
		"history":    func(a []string) error { _, err := parseHistory(a); return err },
		"sync":       func(a []string) error { _, err := parseSync(a); return err },
		"folders":    func(a []string) error { return parseNoArgs("folders", a) },
		"version":    func(a []string) error { return parseNoArgs("version", a) },
		"migrate":    func(a []string) error { return parseNoArgs("migrate", a) },
	}
	for name, parse := range parsers {
		if err := parse([]string{"--color", "never"}); err != nil {
			t.Errorf("%s rejected --color: %v", name, err)
		}
	}
	if len(parsers) != len(commands)-1 { // help takes a topic, not flags
		t.Errorf("this test covers %d commands but there are %d; add the new one",
			len(parsers), len(commands)-1)
	}
}

// A flag that works but is undocumented may as well not exist.
func TestColorFlagIsDocumentedEverywhere(t *testing.T) {
	for _, c := range commands {
		if c.name == "help" {
			continue
		}
		if !strings.Contains(c.usage, "--color") {
			t.Errorf("%s usage does not mention --color", c.name)
		}
	}
}

func TestMigrateCommandExists(t *testing.T) {
	c := lookup("migrate")
	if c == nil {
		t.Fatal("no migrate command registered")
	}
	if c.summary == "" {
		t.Error("migrate has no summary, so `help` would list it blank")
	}
	if !strings.Contains(c.usage, "XDG_CONFIG_HOME") {
		t.Error("migrate usage should explain what it migrates and why")
	}
}
