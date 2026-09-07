package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

// TestRunListWritesToInjectedStdout is the acceptance test for the whole
// refactor: a command's output is observable without spawning a subprocess.
func TestRunListWritesToInjectedStdout(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{
		loc:    disc.Location{Dir: dir},
		stdout: &out,
		stderr: &errOut,
	}

	if err := a.runList(selection{color: term.Never}); err != nil {
		t.Fatalf("runList: %v", err)
	}
	if !strings.Contains(out.String(), "Miles Davis - Kind of Blue") {
		t.Errorf("stdout missing the album, got:\n%s", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr should be empty on success, got: %q", errOut.String())
	}
}

// TestRunListEmptyMatchReturnsErrorAndWritesNothing pins the contract that a
// failing command leaves stdout untouched -- the rule --json depends on.
func TestRunListEmptyMatchReturnsErrorAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: dir}, stdout: &out, stderr: &errOut}

	filter := disc.Filter{Query: disc.FieldFilter{Include: []string{"zzzz-no-such-album"}}}

	err := a.runList(selection{color: term.Never, filter: filter})
	if err == nil {
		t.Fatal("expected an error for an empty match")
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty on failure, got: %q", out.String())
	}
}

// TestRunHistoryEmptyIsNotAnError pins that an empty history prints its notice
// and succeeds. Before the refactor this fact could only be checked by
// re-execing the test binary.
func TestRunHistoryEmptyIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte(`[]`), 0644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: dir}, stdout: &out, stderr: &errOut}

	if err := a.runHistory(historyConfig{color: term.Never}); err != nil {
		t.Fatalf("empty history should succeed, got: %v", err)
	}
	if !strings.Contains(out.String(), "No history yet") {
		t.Errorf("expected the empty-history notice, got: %q", out.String())
	}
}

// TestMissingCollectionCarriesItsGuidance pins the wording a user sees when
// they have not synced yet -- the text Task 4 moved from fatal() into an
// error value.
func TestMissingCollectionCarriesItsGuidance(t *testing.T) {
	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: t.TempDir()}, stdout: &out, stderr: &errOut}

	err := a.runList(selection{color: term.Never})
	if err == nil {
		t.Fatal("expected an error when there is no collection")
	}
	if !strings.Contains(err.Error(), "Run `disc-fortune sync`") {
		t.Errorf("error should tell the user how to fix it, got: %q", err.Error())
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty, got: %q", out.String())
	}
}

func writeTestCollection(t *testing.T, path string) {
	t.Helper()
	const data = `[{"release_id":1,"artist":"Miles Davis","title":"Kind of Blue","year":1959,"label":"Columbia","catno":"CL 1355","genres":["Jazz"],"formats":["Vinyl","LP"]}]`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
}

// TestRunPickAppendsToHistory pins the behaviour reroll must not disturb:
// every pick adds an entry, it never overwrites one.
func TestRunPickAppendsToHistory(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: dir}, stdout: &out, stderr: &errOut}

	for i := 0; i < 3; i++ {
		if err := a.runPick(selection{color: term.Never}); err != nil {
			t.Fatalf("runPick %d: %v", i, err)
		}
	}

	entries, err := disc.LoadHistory(filepath.Join(dir, "history.json"))
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("got %d history entries after 3 picks, want 3", len(entries))
	}
}

// TestRunRerollReplacesRatherThanAppends is the headline behaviour: the
// history entry count does not grow.
func TestRunRerollReplacesRatherThanAppends(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: dir}, stdout: &out, stderr: &errOut}

	if err := a.runPick(selection{color: term.Never}); err != nil {
		t.Fatalf("runPick: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := a.runReroll(selection{color: term.Never}); err != nil {
			t.Fatalf("runReroll %d: %v", i, err)
		}
	}

	entries, err := disc.LoadHistory(filepath.Join(dir, "history.json"))
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d history entries after 1 pick and 3 rerolls, want 1", len(entries))
	}
}

// The proof that the drop precedes the --unheard filter. With a one-album
// collection, a reroll can only succeed if dropping the entry has made that
// album unheard again. If the order were reversed the pool would be empty and
// the command would fail.
func TestRunRerollDropsTheEntryBeforeFiltering(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: dir}, stdout: &out, stderr: &errOut}

	if err := a.runPick(selection{color: term.Never, unheard: true}); err != nil {
		t.Fatalf("runPick --unheard: %v", err)
	}
	if err := a.runReroll(selection{color: term.Never, unheard: true}); err != nil {
		t.Fatalf("runReroll --unheard: %v -- the dropped entry should have made "+
			"the album unheard again", err)
	}
	if !strings.Contains(out.String(), "Kind of Blue") {
		t.Errorf("stdout missing the album, got:\n%s", out.String())
	}
}

func TestRunRerollOnEmptyHistoryFailsAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: dir}, stdout: &out, stderr: &errOut}

	err := a.runReroll(selection{color: term.Never})
	if !errors.Is(err, errNothingToReroll) {
		t.Fatalf("got %v, want errNothingToReroll", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty on failure, got: %q", out.String())
	}
	if _, statErr := os.Stat(filepath.Join(dir, "history.json")); !os.IsNotExist(statErr) {
		t.Errorf("a failed reroll must not create history.json (stat: %v)", statErr)
	}
}

// The receipt goes to stderr and never to stdout, so a script reading stdout
// sees only the pick.
func TestRunRerollReportsTheReplacementOnStderr(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: dir}, stdout: &out, stderr: &errOut}

	if err := a.runPick(selection{color: term.Never}); err != nil {
		t.Fatalf("runPick: %v", err)
	}
	out.Reset()
	errOut.Reset()

	if err := a.runReroll(selection{color: term.Never}); err != nil {
		t.Fatalf("runReroll: %v", err)
	}
	if !strings.Contains(errOut.String(), "Replaced: Miles Davis - Kind of Blue") {
		t.Errorf("stderr missing the replacement receipt, got: %q", errOut.String())
	}
	if strings.Contains(out.String(), "Replaced:") {
		t.Errorf("the receipt leaked into stdout: %q", out.String())
	}
}

// --json changes the format and nothing else: the payload is pick's, with no
// reroll-specific keys.
func TestRunRerollJSONEmitsThePickPayload(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: dir}, stdout: &out, stderr: &errOut}

	if err := a.runPick(selection{color: term.Never}); err != nil {
		t.Fatalf("runPick: %v", err)
	}
	out.Reset()

	if err := a.runReroll(selection{color: term.Never, json: true}); err != nil {
		t.Fatalf("runReroll --json: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshaling stdout %q: %v", out.String(), err)
	}
	if len(payload) != 1 {
		t.Errorf("payload has %d keys (%v), want exactly 1 -- reroll emits pick's payload",
			len(payload), payload)
	}
	if _, ok := payload["album"]; !ok {
		t.Errorf("payload has no \"album\" key: %v", payload)
	}
}
