package main

import (
	"bytes"
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
