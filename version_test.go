package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// moduleMajorPattern matches the /vN suffix Go requires on a module path for
// major versions 2 and up.
var moduleMajorPattern = regexp.MustCompile(`/v(\d+)$`)

// TestVersionMatchesModulePath guards the mistake v2.1.1 shipped to fix: a
// module path claiming one major version while `version` claims another.
// `go install <module>@latest` resolves through the module path, so a
// mismatch hands users something other than what this source tree builds.
//
// This is a test rather than a CI step, following the repo's other
// forcing-function tests. It runs in the same `go test ./...` the workflow
// already invokes, and it fails on the developer's machine before a release
// rather than only on a pull request.
func TestVersionMatchesModulePath(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}

	var modulePath string
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			modulePath = strings.TrimSpace(rest)
			break
		}
	}
	if modulePath == "" {
		t.Fatal("no module line in go.mod")
	}

	major, err := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
	if err != nil {
		t.Fatalf("parsing a major version from version %q: %v", version, err)
	}

	m := moduleMajorPattern.FindStringSubmatch(modulePath)
	if m == nil {
		// No suffix is correct for v0 and v1, which share an unsuffixed path.
		if major > 1 {
			t.Errorf("version %q is major %d but module path %q has no /v%d suffix;\n"+
				"add it to go.mod, or correct version in main.go",
				version, major, modulePath, major)
		}
		return
	}

	want, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("parsing the major version from module path %q: %v", modulePath, err)
	}
	if major != want {
		t.Errorf("version %q is major %d but module path %q says major %d;\n"+
			"bump the /vN suffix in go.mod, or correct version in main.go",
			version, major, modulePath, want)
	}
}
