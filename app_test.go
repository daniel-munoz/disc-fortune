package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunListWritesToInjectedStdout is the acceptance test for the whole
// refactor: a command's output is observable without spawning a subprocess.
func TestRunListWritesToInjectedStdout(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{
		loc:    configLocation{Dir: dir},
		stdout: &out,
		stderr: &errOut,
	}

	if err := a.runList(selection{color: colorNever}); err != nil {
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
	a := app{loc: configLocation{Dir: dir}, stdout: &out, stderr: &errOut}

	filter := Filter{Query: FieldFilter{Include: []string{"zzzz-no-such-album"}}}

	err := a.runList(selection{color: colorNever, filter: filter})
	if err == nil {
		t.Fatal("expected an error for an empty match")
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty on failure, got: %q", out.String())
	}
}

func writeTestCollection(t *testing.T, path string) {
	t.Helper()
	const data = `[{"release_id":1,"artist":"Miles Davis","title":"Kind of Blue","year":1959,"label":"Columbia","catno":"CL 1355","genres":["Jazz"],"formats":["Vinyl","LP"]}]`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
}
