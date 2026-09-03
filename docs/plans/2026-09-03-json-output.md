# `--json` Output (T7) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `pick`, `list` and `history` a `--json` flag that emits a documented, stable machine-readable payload, so a script can read tonight's pick without parsing display text.

**Architecture:** A new `json.go` holds the wire types — deliberately separate from `Album`, so the on-disk format and the output format are two contracts with two owners. Every album key is always present, with `null` for unknown scalars and `[]` for absent lists. Each command emits a JSON object, never a bare array, so a key can be added later without breaking consumers. The three `run*` functions branch on one boolean; nothing about exit codes, stderr, or history side effects changes.

**Tech Stack:** Go 1.24.3, standard library only (`encoding/json`). Single `package main` at the repository root; tests live beside the code as `*_test.go`.

**Spec:** [`docs/plans/2026-09-03-json-output-design.md`](2026-09-03-json-output-design.md)

## Global Constraints

- Module is `github.com/daniel-munoz/disc-fortune/v2`, Go 1.24.3. **No third-party dependencies.** `go.mod` must stay dependency-free.
- Everything is `package main` in the repository root. There is no `src/` and no `tests/` directory.
- Run tests with `go test ./...` from the repository root. A single test is `go test . -run TestName -v`.
- **`--json` changes the format, never the semantics.** Every exit code stays byte-identical to v2.3.0. `TestExitCodes` and `TestFailureDiagnosticsGoToStderr` in `main_test.go` must pass **unchanged** — never by editing what they assert.
- **When a command exits non-zero, stdout stays empty.** No partial payload.
- **JSON is never colourised**, whatever `--color` says. An ANSI escape (`0x1b`) inside a JSON string is a parse hazard.
- **Every album key is always present:** `release_id`, `artist`, `title`, `year`, `label`, `catno`, `genres`, `formats` — in that order. `null` for unknown scalars, `[]` for absent lists. `artist` and `title` are never null.
- **`--json` is registered only on `pick`, `list` and `history`**, following the `--draw` precedent, so `sync --json` fails as an unknown flag.
- `Album`'s existing `omitempty` storage tags must **not** be touched.
- Output is two-space indented (`json.MarshalIndent(v, "", "  ")`) with a trailing newline.
- Run `gofmt -l .` and `go vet ./...` before every commit; both must be clean.
- Comments explain *why*, not *what* — match the density and voice of the surrounding code.
- Commit after every task, using the repo's `type: summary` style, ending with:
```
Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST
```

---

### Task 1: The wire format

Types, conversion and encoder, all standalone. Nothing calls them yet, so the package stays green.

**Files:**
- Create: `json.go`
- Test: `json_test.go`

**Interfaces:**
- Consumes: `Album` and `HistoryEntry` (existing, unmodified).
- Produces:
  - `type jsonAlbum struct` with the eight documented fields
  - `func newJSONAlbum(a Album) jsonAlbum`
  - `type pickPayload struct { Album jsonAlbum \`json:"album"\` }`
  - `type listPayload struct { Albums []jsonAlbum \`json:"albums"\`; Count int \`json:"count"\` }`
  - `type jsonHistoryEntry struct { Album jsonAlbum \`json:"album"\`; Timestamp time.Time \`json:"timestamp"\` }`
  - `type historyPayload struct { Entries []jsonHistoryEntry \`json:"entries"\`; Count int \`json:"count"\` }`
  - `func newListPayload(albums []Album) listPayload`
  - `func newHistoryPayload(entries []HistoryEntry, limit int) historyPayload`
  - `func writeJSON(w io.Writer, v any) error`

- [ ] **Step 1: Write the failing test**

Create `json_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The golden tests below pin the exact bytes of the wire format. They are
// what stops it drifting when Album changes: a field added to storage
// without a decision about the output fails here rather than silently
// altering what every script sees.

func TestJSONAlbumGoldenFullyPopulated(t *testing.T) {
	album := Album{
		ReleaseID: 1839278,
		Artist:    "Miles Davis",
		Title:     "Kind of Blue",
		Year:      1959,
		Label:     "Columbia",
		CatNo:     "CL 1355",
		Genres:    []string{"Jazz"},
		Formats:   []string{"Vinyl", "LP", "Album"},
	}

	want := `{
  "release_id": 1839278,
  "artist": "Miles Davis",
  "title": "Kind of Blue",
  "year": 1959,
  "label": "Columbia",
  "catno": "CL 1355",
  "genres": [
    "Jazz"
  ],
  "formats": [
    "Vinyl",
    "LP",
    "Album"
  ]
}
`

	var buf bytes.Buffer
	if err := writeJSON(&buf, newJSONAlbum(album)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("wire format drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// An album Discogs told us almost nothing about still carries all eight
// keys. null says "we were not told", which "" and 0 cannot: "year": 0
// sorts before 1959 and "release_id": 0 looks like an ID.
func TestJSONAlbumGoldenEverythingAbsent(t *testing.T) {
	album := Album{Artist: "Some Artist", Title: "Untitled"}

	want := `{
  "release_id": null,
  "artist": "Some Artist",
  "title": "Untitled",
  "year": null,
  "label": null,
  "catno": null,
  "genres": [],
  "formats": []
}
`

	var buf bytes.Buffer
	if err := writeJSON(&buf, newJSONAlbum(album)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("wire format drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPickPayloadGolden(t *testing.T) {
	album := Album{ReleaseID: 42, Artist: "A", Title: "B"}

	want := `{
  "album": {
    "release_id": 42,
    "artist": "A",
    "title": "B",
    "year": null,
    "label": null,
    "catno": null,
    "genres": [],
    "formats": []
  }
}
`

	var buf bytes.Buffer
	if err := writeJSON(&buf, pickPayload{Album: newJSONAlbum(album)}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("wire format drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestListPayloadCountsWhatItEmits(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1"},
		{Artist: "B", Title: "2"},
	}
	got := newListPayload(albums)
	if got.Count != 2 {
		t.Errorf("Count = %d, want 2", got.Count)
	}
	if len(got.Albums) != 2 {
		t.Fatalf("Albums has %d entries, want 2", len(got.Albums))
	}
	if got.Albums[0].Artist != "A" || got.Albums[1].Artist != "B" {
		t.Errorf("albums out of order: %q, %q", got.Albums[0].Artist, got.Albums[1].Artist)
	}
}

// An empty list must marshal as [], never null: a consumer's loop should
// need no nil check.
func TestListPayloadEmptyIsAnEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, newListPayload(nil)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	want := `{
  "albums": [],
  "count": 0
}
`
	if got := buf.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Entries come back most recent first, matching what formatHistory prints,
// and count is how many were emitted rather than how many the file holds.
func TestHistoryPayloadIsMostRecentFirst(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{Album: Album{Artist: "oldest", Title: "1"}, Timestamp: base},
		{Album: Album{Artist: "middle", Title: "2"}, Timestamp: base.Add(time.Hour)},
		{Album: Album{Artist: "newest", Title: "3"}, Timestamp: base.Add(2 * time.Hour)},
	}

	got := newHistoryPayload(entries, 2)
	if got.Count != 2 {
		t.Errorf("Count = %d, want 2", got.Count)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("Entries has %d, want 2", len(got.Entries))
	}
	if got.Entries[0].Album.Artist != "newest" {
		t.Errorf("Entries[0] = %q, want newest", got.Entries[0].Album.Artist)
	}
	if got.Entries[1].Album.Artist != "middle" {
		t.Errorf("Entries[1] = %q, want middle", got.Entries[1].Album.Artist)
	}
}

// A limit larger than the history, or zero, means "all of it" -- the same
// clamping formatHistory does.
func TestHistoryPayloadClampsLimit(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{Album: Album{Artist: "a", Title: "1"}, Timestamp: base},
		{Album: Album{Artist: "b", Title: "2"}, Timestamp: base.Add(time.Hour)},
	}

	for _, limit := range []int{0, 2, 99, -1} {
		got := newHistoryPayload(entries, limit)
		if got.Count != 2 {
			t.Errorf("limit %d: Count = %d, want 2", limit, got.Count)
		}
	}
}

func TestHistoryPayloadEmptyIsAnEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, newHistoryPayload(nil, 0)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	want := `{
  "entries": [],
  "count": 0
}
`
	if got := buf.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The timestamp on the wire and the timestamp in history.json must be the
// same string, so no rounding can make them disagree.
func TestHistoryPayloadTimestampIsRFC3339AsStored(t *testing.T) {
	ts := time.Date(2026, 9, 3, 21, 45, 6, 123456789, time.UTC)
	entries := []HistoryEntry{{Album: Album{Artist: "a", Title: "1"}, Timestamp: ts}}

	var buf bytes.Buffer
	if err := writeJSON(&buf, newHistoryPayload(entries, 1)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"2026-09-03T21:45:06.123456789Z"`) {
		t.Errorf("timestamp not RFC 3339 as stored:\n%s", buf.String())
	}
}

// Output is not merely plausible: it parses.
func TestWriteJSONRoundTrips(t *testing.T) {
	albums := []Album{
		{ReleaseID: 1, Artist: "A", Title: "1", Year: 1970, Genres: []string{"Jazz"}},
		{Artist: "B", Title: "2"},
	}

	var buf bytes.Buffer
	if err := writeJSON(&buf, newListPayload(albums)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}

	var back listPayload
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatalf("emitted JSON does not parse: %v\n%s", err, buf.String())
	}
	if back.Count != 2 || len(back.Albums) != 2 {
		t.Errorf("round trip lost data: %+v", back)
	}
	if back.Albums[0].ReleaseID == nil || *back.Albums[0].ReleaseID != 1 {
		t.Errorf("release_id did not survive the round trip: %+v", back.Albums[0])
	}
	if back.Albums[1].ReleaseID != nil {
		t.Errorf("absent release_id should stay null, got %v", *back.Albums[1].ReleaseID)
	}
}

func TestWriteJSONEndsWithExactlyOneNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, pickPayload{Album: newJSONAlbum(Album{Artist: "A", Title: "B"})}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	out := buf.String()
	if !strings.HasSuffix(out, "}\n") || strings.HasSuffix(out, "\n\n") {
		t.Errorf("want output ending in exactly one newline, got %q", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestJSONAlbum|TestPickPayload|TestListPayload|TestHistoryPayload|TestWriteJSON' -v`
Expected: compile failure — `undefined: writeJSON`, `undefined: newJSONAlbum`.

- [ ] **Step 3: Write the implementation**

Create `json.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// jsonAlbum is the wire representation of an Album. It is deliberately a
// separate type: the on-disk format and the machine-readable output are two
// contracts with two owners, and serialising the storage struct directly
// would make every future storage change a breaking change for anyone
// scripting against the output.
//
// Every key is always present, so a consumer can model a fixed type and tell
// a missing value from a typo. A nil pointer marshals to null, which says
// "Discogs did not tell us" -- something "" and 0 cannot: "year": 0 sorts
// before 1959, and "release_id": 0 looks like an ID.
type jsonAlbum struct {
	ReleaseID *int     `json:"release_id"`
	Artist    string   `json:"artist"`
	Title     string   `json:"title"`
	Year      *int     `json:"year"`
	Label     *string  `json:"label"`
	CatNo     *string  `json:"catno"`
	Genres    []string `json:"genres"`
	Formats   []string `json:"formats"`
}

// newJSONAlbum converts a stored Album to its wire form. Artist and Title are
// never null: they are the one pair every entry has, and Album.Key() -- the
// identity for anything written before release IDs existed -- is built from
// them.
func newJSONAlbum(a Album) jsonAlbum {
	return jsonAlbum{
		ReleaseID: intOrNull(a.ReleaseID),
		Artist:    a.Artist,
		Title:     a.Title,
		Year:      intOrNull(a.Year),
		Label:     stringOrNull(a.Label),
		CatNo:     stringOrNull(a.CatNo),
		Genres:    listOrEmpty(a.Genres),
		Formats:   listOrEmpty(a.Formats),
	}
}

func intOrNull(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func stringOrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// listOrEmpty turns a nil slice into an empty one, so a record with no genres
// emits [] rather than null and a consumer's loop needs no nil check.
func listOrEmpty(vals []string) []string {
	if vals == nil {
		return []string{}
	}
	return vals
}

// The payloads below are objects rather than bare arrays for one reason: a key
// can be added to an object without breaking a consumer, and a top-level array
// can never become an object. For a schema meant to be permanent, that
// asymmetry decides it.

// pickPayload is `pick --json`.
type pickPayload struct {
	Album jsonAlbum `json:"album"`
}

// listPayload is `list --json`. Count is how many albums were emitted.
type listPayload struct {
	Albums []jsonAlbum `json:"albums"`
	Count  int         `json:"count"`
}

// jsonHistoryEntry pairs an album with when it was picked. The timestamp is
// RFC 3339 exactly as stored, so the wire value and the history.json value are
// the same string.
type jsonHistoryEntry struct {
	Album     jsonAlbum `json:"album"`
	Timestamp time.Time `json:"timestamp"`
}

// historyPayload is `history --json`. Count is how many entries were emitted,
// not how many the file holds.
type historyPayload struct {
	Entries []jsonHistoryEntry `json:"entries"`
	Count   int                `json:"count"`
}

func newListPayload(albums []Album) listPayload {
	out := make([]jsonAlbum, 0, len(albums))
	for _, a := range albums {
		out = append(out, newJSONAlbum(a))
	}
	return listPayload{Albums: out, Count: len(out)}
}

// newHistoryPayload returns the last limit entries, most recent first -- the
// same records formatHistory prints, in the same order. entries arrives in
// storage order, oldest first. The clamping mirrors formatHistory's, so the
// two can never disagree about what "the last N picks" means.
func newHistoryPayload(entries []HistoryEntry, limit int) historyPayload {
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	out := make([]jsonHistoryEntry, 0, limit)
	for i := len(entries) - 1; i >= len(entries)-limit; i-- {
		out = append(out, jsonHistoryEntry{
			Album:     newJSONAlbum(entries[i].Album),
			Timestamp: entries[i].Timestamp,
		})
	}
	return historyPayload{Entries: out, Count: len(out)}
}

// writeJSON emits v as two-space indented JSON with a trailing newline:
// readable without jq installed, and jq normalises anyway.
//
// Nothing here consults the colour mode. An ANSI escape inside a JSON string
// is a parse hazard for no benefit, so --color has no effect on this path.
func writeJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -run 'TestJSONAlbum|TestPickPayload|TestListPayload|TestHistoryPayload|TestWriteJSON' -v`
Expected: PASS, all eleven tests.

If a golden test fails on whitespace, **do not edit the golden to match the output** — read the diff and confirm `json.MarshalIndent(v, "", "  ")` is what produced it. The goldens in Step 1 are the specified format.

- [ ] **Step 5: Run the whole suite**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: PASS, clean, no output from gofmt.

- [ ] **Step 6: Commit**

```bash
git add json.go json_test.go
git commit -m "feat: add the --json wire format

A jsonAlbum separate from Album, so the on-disk format and the output
format are two contracts with two owners. Every key is always present,
with null for unknown scalars and [] for absent lists: a consumer can
model a fixed type, and null says 'Discogs did not tell us' where 0
would look like a real ID.

Golden tests pin the exact bytes, so a storage change cannot silently
alter what every script sees.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 2: The `--json` flag

Registration, usage text, and the guard that stops it shipping undocumented. Nothing reads the flag yet.

**Files:**
- Modify: `cli.go` — `selection`, `historyConfig`, `parseSelection`, `parseHistory`, the `pick`/`list`/`history` usage blocks
- Test: `cli_test.go`, `global_flags_test.go`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `selection.json bool` and `historyConfig.json bool`, set by `--json` on `pick`, `list` and `history`.

- [ ] **Step 1: Write the failing test**

Append to `cli_test.go`:

```go
func TestParseSelectionJSONFlag(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		cfg, err := parseSelection(name, []string{"--json"})
		if err != nil {
			t.Fatalf("parseSelection(%s): %v", name, err)
		}
		if !cfg.json {
			t.Errorf("%s: cfg.json = false, want true", name)
		}
	}
}

func TestParseSelectionJSONDefaultsOff(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		cfg, err := parseSelection(name, nil)
		if err != nil {
			t.Fatalf("parseSelection(%s): %v", name, err)
		}
		if cfg.json {
			t.Errorf("%s: cfg.json = true, want false by default", name)
		}
	}
}

func TestParseHistoryJSONFlag(t *testing.T) {
	cfg, err := parseHistory([]string{"--json", "5"})
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if !cfg.json {
		t.Error("cfg.json = false, want true")
	}
	if cfg.limit != 5 {
		t.Errorf("limit = %d, want 5 (the positional must still work)", cfg.limit)
	}
}

// --json is registered where it is implemented, exactly as --draw is, so a
// command that cannot honour it says so rather than accepting and ignoring it.
func TestJSONFlagRejectedWhereNotImplemented(t *testing.T) {
	if _, err := parseSync([]string{"--json"}); err == nil {
		t.Error("sync accepted --json, want an unknown-flag error")
	}
	if _, err := parseFavorite("favorite", []string{"miles", "--json"}); err == nil {
		t.Error("favorite accepted --json, want an unknown-flag error")
	}
	if _, err := parseFavorite("unfavorite", []string{"miles", "--json"}); err == nil {
		t.Error("unfavorite accepted --json, want an unknown-flag error")
	}
	for _, name := range []string{"folders", "migrate", "version"} {
		if err := parseNoArgs(name, []string{"--json"}); err == nil {
			t.Errorf("%s accepted --json, want an unknown-flag error", name)
		}
	}
}
```

Append to `global_flags_test.go`:

```go
// The commands that accept --json must document it, and the ones that do not
// must not claim to. Same guard as TestUnheardFlagIsDocumentedWhereAccepted.
func TestJSONFlagIsDocumentedWhereAccepted(t *testing.T) {
	for _, name := range []string{"pick", "list", "history"} {
		c := lookup(name)
		if c == nil {
			t.Fatalf("command %q not found", name)
		}
		if !strings.Contains(c.usage, "--json") {
			t.Errorf("%s usage does not mention --json", name)
		}
	}
	for _, name := range []string{"favorite", "unfavorite", "sync", "folders", "migrate", "version", "help"} {
		c := lookup(name)
		if c == nil {
			continue
		}
		if strings.Contains(c.usage, "--json") {
			t.Errorf("%s documents --json but does not accept it", name)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestParseSelectionJSON|TestParseHistoryJSON|TestJSONFlag' -v`
Expected: failures — `cfg.json undefined`, and the documentation guard failing.

- [ ] **Step 3: Add the flag to the config structs**

In `cli.go`, add to `selection`:

```go
	// json switches the data channel to the documented machine-readable
	// payload. It changes the format only: exit codes, stderr advice and
	// history side effects are identical either way.
	json bool
```

and to `historyConfig`:

```go
	json bool
```

- [ ] **Step 4: Register the flag**

In `parseSelection`, beside the `--unheard` registration (it applies to both `pick` and `list`, unlike `--draw`):

```go
	asJSON := fs.Bool("json", false, "Emit machine-readable JSON instead of text")
```

and set it in the returned `selection`:

```go
		json:          *asJSON,
```

In `parseHistory`, after `newFlagSet`:

```go
	asJSON := fs.Bool("json", false, "Emit machine-readable JSON instead of text")
```

and set `json: *asJSON` in the returned `historyConfig`.

- [ ] **Step 5: Document it in the three usage blocks**

`pick`, in its `Flags:` list before `filterFlagHelp`:

```
  --json           Emit machine-readable JSON instead of text
```

`list`, in the same place. `history`, whose usage block has no `Flags:` section yet, gains one:

```go
			usage: `Usage: disc-fortune history [N] [flags]

Shows the last N picks. N defaults to 10; 0 shows all of them.

Flags:
  --json           Emit machine-readable JSON instead of text`,
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test . -run 'TestParseSelectionJSON|TestParseHistoryJSON|TestJSONFlag' -v`
Expected: PASS.

- [ ] **Step 7: Run the whole suite**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: PASS, clean. `TestUsageBlocksHaveNoDoubleBlankLines` must still pass — check the `history` usage block did not gain a stray blank line.

- [ ] **Step 8: Commit**

```bash
git add cli.go cli_test.go global_flags_test.go
git commit -m "feat: register --json on pick, list and history

Registered where it is implemented, following --draw: sync --json fails
as an unknown flag rather than being accepted and silently ignored. A
guard pins which commands document it and which must not claim to.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 3: Wire the three commands

Where the flag starts doing something. The load-bearing property is that nothing else changes.

**Files:**
- Modify: `main.go` — `runPick`, `runList`, `runHistory`
- Test: `main_test.go`

**Interfaces:**
- Consumes: `writeJSON`, `pickPayload`, `newJSONAlbum`, `newListPayload`, `newHistoryPayload` (Task 1); `cfg.json` (Task 2).
- Produces: nothing.

- [ ] **Step 1: Write the failing test**

Append to `main_test.go`. These use the existing subprocess harness (`runHelperSplit`, `fixturePaths`, `mustSaveCollection`, `mustSaveHistory`), which runs the real binary against a throwaway `HOME`:

```go
// TestJSONOutput drives the real binary and parses what it emits. A payload
// that only looks right is not enough -- these decode it.
func TestJSONOutput(t *testing.T) {
	miles := Album{ReleaseID: 1839278, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959, Label: "Columbia", Genres: []string{"Jazz"}}
	bare := Album{Artist: "Some Artist", Title: "Untitled"}

	t.Run("pick emits one album and exits 0", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		code, stdout, _ := runHelperSplit(t, home, "pick", "--json")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		var got pickPayload
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout does not parse: %v\n%s", err, stdout)
		}
		if got.Album.Artist != "Miles Davis" {
			t.Errorf("artist = %q, want Miles Davis", got.Album.Artist)
		}
		if got.Album.ReleaseID == nil || *got.Album.ReleaseID != 1839278 {
			t.Errorf("release_id missing from the payload: %+v", got.Album)
		}
	})

	t.Run("pick still records history", func(t *testing.T) {
		home := t.TempDir()
		collection, _, historyFile := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		if code, _, stderr := runHelperSplit(t, home, "pick", "--json"); code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
		}
		entries, err := loadHistory(historyFile)
		if err != nil {
			t.Fatalf("loadHistory: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("history has %d entries, want 1 -- --json is a format flag, not a dry run", len(entries))
		}
	})

	t.Run("list emits albums and a count", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles, bare})

		code, stdout, _ := runHelperSplit(t, home, "list", "--json")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		var got listPayload
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout does not parse: %v\n%s", err, stdout)
		}
		if got.Count != 2 || len(got.Albums) != 2 {
			t.Errorf("count = %d, albums = %d, want 2 and 2", got.Count, len(got.Albums))
		}
	})

	t.Run("an album with nothing known still carries every key", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{bare})

		_, stdout, _ := runHelperSplit(t, home, "list", "--json")
		for _, key := range []string{`"release_id"`, `"artist"`, `"title"`, `"year"`, `"label"`, `"catno"`, `"genres"`, `"formats"`} {
			if !strings.Contains(stdout, key) {
				t.Errorf("payload is missing %s:\n%s", key, stdout)
			}
		}
		if !strings.Contains(stdout, `"genres": []`) {
			t.Errorf("absent genres should be [], not null:\n%s", stdout)
		}
	})

	t.Run("history is most recent first with a count", func(t *testing.T) {
		home := t.TempDir()
		collection, _, historyFile := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})
		base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
		mustSaveHistory(t, historyFile, []HistoryEntry{
			{Album: Album{Artist: "oldest", Title: "1"}, Timestamp: base},
			{Album: Album{Artist: "middle", Title: "2"}, Timestamp: base.Add(time.Hour)},
			{Album: Album{Artist: "newest", Title: "3"}, Timestamp: base.Add(2 * time.Hour)},
		})

		code, stdout, _ := runHelperSplit(t, home, "history", "--json", "2")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		var got historyPayload
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout does not parse: %v\n%s", err, stdout)
		}
		if got.Count != 2 {
			t.Errorf("count = %d, want 2 (what was emitted, not what the file holds)", got.Count)
		}
		if len(got.Entries) != 2 || got.Entries[0].Album.Artist != "newest" {
			t.Fatalf("entries not most-recent-first: %+v", got.Entries)
		}
	})
}

// TestJSONDoesNotChangeSemantics is the load-bearing test of this task.
// --json is a formatting flag: every exit code and every stream stays as it
// was, so anyone scripting today keeps working.
func TestJSONDoesNotChangeSemantics(t *testing.T) {
	miles := Album{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959}

	t.Run("list matching nothing still exits 1 with an empty stdout", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		code, stdout, stderr := runHelperSplit(t, home, "list", "--json", "--year", "1899")
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty -- no partial payload on a failing exit", stdout)
		}
		if !strings.Contains(stderr, "No albums match the specified filters") {
			t.Errorf("stderr = %q, want the no-match message", stderr)
		}
	})

	t.Run("pick matching nothing still exits 1 with an empty stdout", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		code, stdout, _ := runHelperSplit(t, home, "pick", "--json", "--year", "1899")
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
	})

	// history and list disagree about whether an empty result is a failure.
	// That predates this task; the JSON mirrors it rather than reconciling
	// it, because changing either would be a silent change to a scripted
	// exit code.
	t.Run("history on an empty history exits 0 with an empty payload", func(t *testing.T) {
		home := t.TempDir()
		collection, _, historyFile := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})
		mustSaveHistory(t, historyFile, nil)

		code, stdout, _ := runHelperSplit(t, home, "history", "--json")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		var got historyPayload
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout does not parse: %v\n%s", err, stdout)
		}
		if got.Count != 0 || len(got.Entries) != 0 {
			t.Errorf("want an empty payload, got %+v", got)
		}
	})

	// An ANSI escape inside a JSON string would be a parse hazard, so the
	// colour mode has no effect on this path.
	t.Run("--color=always injects no escapes", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		for _, cmd := range [][]string{
			{"pick", "--json", "--color", "always"},
			{"list", "--json", "--color", "always"},
			{"history", "--json", "--color", "always"},
		} {
			_, stdout, _ := runHelperSplit(t, home, cmd...)
			if strings.ContainsRune(stdout, 0x1b) {
				t.Errorf("%v: stdout contains an ANSI escape:\n%q", cmd, stdout)
			}
		}
	})
}
```

`main_test.go` already imports `time`, `strings` and `os`; it needs `encoding/json` added to its import block.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestJSONOutput|TestJSONDoesNotChangeSemantics' -v`
Expected: failures — the commands ignore `--json` and print human text, so `json.Unmarshal` fails.

- [ ] **Step 3: Wire `runPick`**

Replace the single print line in `runPick`:

```go
	if cfg.json {
		if err := writeJSON(os.Stdout, pickPayload{Album: newJSONAlbum(album)}); err != nil {
			fatal("Error writing JSON: %v", err)
		}
	} else {
		fmt.Println(formatAlbum(album, stdoutColor(cfg.color)))
	}
```

Leave the `syncNotice` call after it exactly as it is — it is advisory, already on stderr, and `--json` does not imply quiet.

- [ ] **Step 4: Wire `runList`**

Insert before the existing `out := formatList(...)` line:

```go
	// The empty case below is deliberately left alone: an empty list has
	// always been a failure, with its message on stderr and exit 1. --json
	// changes the format, not the semantics.
	if cfg.json && len(albums) > 0 {
		if err := writeJSON(os.Stdout, newListPayload(albums)); err != nil {
			fatal("Error writing JSON: %v", err)
		}
		return
	}
```

- [ ] **Step 5: Wire `runHistory`**

After the existing `limit` resolution and before the `fmt.Print(formatHistory(...))` line:

```go
	if cfg.json {
		if err := writeJSON(os.Stdout, newHistoryPayload(entries, limit)); err != nil {
			fatal("Error writing JSON: %v", err)
		}
		return
	}
```

- [ ] **Step 6: Run the new tests**

Run: `go test . -run 'TestJSONOutput|TestJSONDoesNotChangeSemantics' -v`
Expected: PASS, all nine subtests.

- [ ] **Step 7: Confirm the pre-existing behaviour tests are untouched and still pass**

Run: `go test . -run 'TestExitCodes|TestFailureDiagnosticsGoToStderr' -v`
Expected: PASS, **without either test having been edited**. If either needs changing to pass, stop and report it: it would mean `--json` changed semantics, which this task must not do.

- [ ] **Step 8: Run the whole suite and try the real binary**

```bash
go test ./... && go vet ./... && gofmt -l .
go build -o /tmp/df-t7 . && /tmp/df-t7 help history
```
Expected: tests PASS, clean; `help history` shows the `--json` flag.

- [ ] **Step 9: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: emit JSON from pick, list and history

--json switches the data channel and nothing else: exit codes, stderr
advice and pick's history write are identical either way, so anyone
scripting today's behaviour keeps working. list and history disagree
about whether an empty result is a failure; the JSON mirrors that rather
than reconciling it, because changing either would be a silent change to
a scripted exit code.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 4: Document the schema

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing.

- [ ] **Step 1: Add a `--json` section**

Place it after the `### Filtering` section and before the next `### ` heading. **Check the surrounding code fences first** — the Usage examples live inside a ` ```sh ` block, so confirm with `grep -n '^```' README.md` that your new section sits outside any open fence, and that every fence still pairs.

````markdown
### JSON output

`pick`, `list` and `history` accept `--json`, which replaces the human
output with a documented payload. Nothing else changes: exit codes, the
messages on stderr, and `pick` recording its pick are all identical either way.

```sh
disc-fortune pick --json
disc-fortune list --json --genre jazz
disc-fortune history --json 5
```

Each command emits a single JSON object:

```json
{
  "album": {
    "release_id": 1839278,
    "artist": "Miles Davis",
    "title": "Kind of Blue",
    "year": 1959,
    "label": "Columbia",
    "catno": "CL 1355",
    "genres": ["Jazz"],
    "formats": ["Vinyl", "LP", "Album"]
  }
}
```

`list` emits `{"albums": [...], "count": N}` and `history` emits
`{"entries": [{"album": {...}, "timestamp": "..."}], "count": N}`, most recent
first. `count` is how many records were emitted, so `history --json 5` reports
at most `5` — fewer if your history is shorter.

Every album key is always present. `release_id`, `year`, `label` and `catno`
are `null` when Discogs did not say — `release_id` is also `null` for anything
picked before v2.2.0 — while `genres` and `formats` are `[]` rather than null,
so a loop over them needs no guard. `artist` and `title` are always strings.

Exit codes are unchanged, which means a script should check them: `list --json`
matching nothing writes its message to stderr and exits 1 with an empty stdout,
rather than emitting an empty result.

```sh
if out=$(disc-fortune list --json --genre jazz); then
  echo "$out" | jq -r '.albums[] | "\(.artist) - \(.title)"'
fi
```
````

- [ ] **Step 2: Update the feature list**

Add a bullet beside the existing ones:

```markdown
- **Scriptable** - `--json` on `pick`, `list` and `history` emits a documented payload with a fixed key set
```

- [ ] **Step 3: Verify the fences and the documented commands**

```bash
grep -n '^```' README.md
go build -o /tmp/df-t7 .
/tmp/df-t7 --help >/dev/null && echo "binary ok"
```
Read the fence sequence and confirm every opening fence has a matching close, that none is nested, and that `### JSON output` sits outside any fence. Then confirm each command shown in the new section actually runs, using a throwaway `HOME` so nothing touches real data:

```bash
export TMPHOME=$(mktemp -d)
mkdir -p "$TMPHOME/.config/disc-fortune"
printf '[{"release_id":1,"artist":"Miles Davis","title":"Kind of Blue","year":1959,"genres":["Jazz"]}]' > "$TMPHOME/.config/disc-fortune/collection.json"
HOME=$TMPHOME /tmp/df-t7 list --json --genre jazz
HOME=$TMPHOME /tmp/df-t7 pick --json
HOME=$TMPHOME /tmp/df-t7 history --json 5
rm -rf "$TMPHOME"
```
Expected: three parseable JSON objects. **Never run these against your real `HOME`** — `pick` writes to `history.json`.

- [ ] **Step 4: Run the whole suite**

Run: `go test ./...`
Expected: PASS (no Go files changed in this task).

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: document the --json schema

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

## What this plan does not do

- **No release.** v2.4.0 "Composability" ships after T8 (shell completion) lands. No version bump, no release notes here.
- **No `--json` on `favorite`, `unfavorite`, `sync`, `folders`, `migrate` or `help`.** The roadmap names three commands.
- **No draw metadata on `pick`**, no `schema_version` key, and no echo of the filters that produced a result. All three are argued down in the design's §5.
- **No change to `Album`'s storage tags.** The wire format is a separate type on purpose.
