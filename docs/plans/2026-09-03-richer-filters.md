# Richer Filters (T6) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every narrowing filter multiple values that OR together, an `--exclude-NAME` twin that removes matches, and three new fields — `--artist`, `--title`, `--decade` — plus `--query` as a real flag, so `pick` and `list` gain the free-text search they have never had.

**Architecture:** `Filter`'s scalar string fields become `FieldFilter{Include, Exclude}` (substring) and `YearFilter{Include, Exclude}` over parsed `yearRange`s (numeric). A package-level `filterFields` table is the single source of truth for the six substring filters — flag name, help line, how to read the album, where parsed values land — and flag registration, help generation and matching all loop over it. Year sits beside the table rather than in it, because it parses its values and two flag names feed it.

**Tech Stack:** Go 1.24.3, standard library only. Single `package main` at the repository root; tests live beside the code as `*_test.go`.

**Spec:** [`docs/plans/2026-09-03-richer-filters-design.md`](2026-09-03-richer-filters-design.md)

## Global Constraints

- Module is `github.com/daniel-munoz/disc-fortune/v2`, Go 1.24.3. **No third-party dependencies.** `go.mod` must stay dependency-free.
- Everything is `package main` in the repository root. There is no `src/` and no `tests/` directory.
- Run tests with `go test .` from the repository root. A single test is `go test . -run TestName -v`.
- **The grammar is one sentence:** values within a field OR together; different fields AND together; any `--exclude-*` match removes the album outright.
- **Absence is not a match.** An album with a zero `Year` or an empty `Label` is never removed by an exclusion. `Year` and `Label` are `omitempty` on `Album` (`collection.go:24-25`) because Discogs often omits them; the opposite rule would let one exclusion delete every such record.
- **Empty flag values are dropped at parse time**, not at match time. `--genre "$GENRE"` with an unset variable must keep meaning "no genre filter", and `strings.Contains(anything, "")` is true, so an empty exclusion that reached the matcher would exclude the entire collection.
- **`--release-id` is untouched.** It identifies one record rather than narrowing a query; it stays a single `int` compared whole, and stays outside `anyNarrowing()`.
- `Album.Key()` must keep returning exactly `Artist + " - " + Title`. It is what `--query` substring-matches against.
- Every filter flag must be documented or be a documented flag's `--exclude-` twin. `TestFilterFlagsAreDocumented` (`global_flags_test.go:133`) enforces it and gets stricter in Task 6.
- Comments explain *why*, not *what* — match the density and voice of the surrounding code.
- Commit after every task, using the repo's `type: summary` message style (`feat:`, `fix:`, `test:`, `refactor:`).
- **No release task here.** v2.4.0 ships after T7 (`--json`) and T8 (completion) also land.

---

### Task 1: `FieldFilter` and its matcher

The substring-matching engine, standalone. Nothing consumes it yet, so the package stays green throughout.

**Files:**
- Modify: `filter.go`
- Test: `filter_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type FieldFilter struct { Include, Exclude []string }`
  - `func (ff FieldFilter) matches(values []string) bool`
  - `func containsAny(values []string, needle string) bool`
  - Test helpers `include(vals ...string) FieldFilter` and `exclude(vals ...string) FieldFilter`, used by every later task's tests.

- [ ] **Step 1: Write the failing test**

Append to `filter_test.go`:

```go
// include and exclude keep the filter literals in these tests readable.
// Production code never builds a FieldFilter by hand -- addFilterFlags does
// it from the flag values.
func include(vals ...string) FieldFilter { return FieldFilter{Include: vals} }
func exclude(vals ...string) FieldFilter { return FieldFilter{Exclude: vals} }

func TestFieldFilterMatches(t *testing.T) {
	tests := []struct {
		name   string
		ff     FieldFilter
		values []string
		want   bool
	}{
		{"unconstrained matches anything", FieldFilter{}, []string{"Jazz"}, true},
		{"include hit", include("jazz"), []string{"Jazz"}, true},
		{"include miss", include("funk"), []string{"Jazz"}, false},
		{"include is OR", include("funk", "jazz"), []string{"Jazz"}, true},
		{"include hits any album value", include("bebop"), []string{"Jazz", "Bebop"}, true},
		{"include is a substring, not a whole value", include("azz"), []string{"Jazz"}, true},
		{"include is case-insensitive", include("JAZZ"), []string{"jazz"}, true},
		{"exclude hit", exclude("rock"), []string{"Rock"}, false},
		{"exclude miss", exclude("rock"), []string{"Jazz"}, true},
		{"exclude hits any album value", exclude("rock"), []string{"Rock", "Jazz"}, false},
		{"exclude is OR", exclude("rock", "pop"), []string{"Pop"}, false},
		{"exclusion beats inclusion", FieldFilter{Include: []string{"jazz"}, Exclude: []string{"jazz"}}, []string{"Jazz"}, false},
		{"an empty album value is excluded by nothing", exclude("blue note"), []string{""}, true},
		{"no album values are excluded by nothing", exclude("rock"), nil, true},
		{"no album values satisfy no inclusion", include("jazz"), nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ff.matches(tt.values); got != tt.want {
				t.Errorf("matches(%q) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test . -run TestFieldFilterMatches -v`
Expected: compile failure — `undefined: FieldFilter`.

- [ ] **Step 3: Write the implementation**

Add to `filter.go`, above the existing `Filter` type:

```go
// FieldFilter is one field's constraint: values to require (any of), and
// values that disqualify. Both empty means unconstrained.
type FieldFilter struct {
	Include []string
	Exclude []string
}

// matches reports whether an album passes this field's constraint. values is
// what the album has for the field: a one-element slice for a scalar like
// Label, every element for a list like Genres.
//
// Exclusion is checked first and wins outright, so --genre jazz
// --exclude-genre jazz is an empty filter rather than a conflict to resolve.
// An album with nothing in this field matches no value, and so is excluded by
// nothing -- absence is not a match, which is what stops one --exclude-label
// from deleting every record Discogs left unlabelled.
func (ff FieldFilter) matches(values []string) bool {
	for _, ex := range ff.Exclude {
		if containsAny(values, ex) {
			return false
		}
	}
	if len(ff.Include) == 0 {
		return true
	}
	for _, in := range ff.Include {
		if containsAny(values, in) {
			return true
		}
	}
	return false
}

// containsAny reports whether needle is a case-insensitive substring of any
// of values. An empty needle matches everything, which is why empty flag
// values are dropped at parse time rather than defended against here.
func containsAny(values []string, needle string) bool {
	needle = strings.ToLower(needle)
	for _, v := range values {
		if strings.Contains(strings.ToLower(v), needle) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test . -run TestFieldFilterMatches -v`
Expected: PASS, every subtest.

- [ ] **Step 5: Run the whole suite**

Run: `go test ./...`
Expected: PASS. Nothing consumes `FieldFilter` yet, so no existing test changes behavior.

- [ ] **Step 6: Commit**

```bash
git add filter.go filter_test.go
git commit -m "feat: add FieldFilter, the include/exclude substring matcher

Values within a field OR together and any exclusion wins outright. An
album with nothing in the field is excluded by nothing: absence is not a
match, which keeps one --exclude-label from deleting every record
Discogs left unlabelled.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 2: `yearRange`, `parseYearValue`, `YearFilter`

The numeric half of the engine. Still standalone: the existing `ParseYearFilter` and `matchesYear` stay in place until Task 4 retires them.

**Files:**
- Modify: `filter.go`
- Test: `filter_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type yearRange struct{ start, end int }` with `func (r yearRange) contains(year int) bool`
  - `type YearFilter struct { Include, Exclude []yearRange }` with `func (yf YearFilter) matches(year int) bool`
  - `func parseYearValue(s string) (yearRange, error)`
  - `var errBadYearFormat error`
  - Test helper `years(t *testing.T, vals ...string) YearFilter`

- [ ] **Step 1: Write the failing test**

Append to `filter_test.go`:

```go
// years builds an inclusive YearFilter from --year spellings, so the tests
// below read the way the command line does.
func years(t *testing.T, vals ...string) YearFilter {
	t.Helper()
	var yf YearFilter
	for _, v := range vals {
		r, err := parseYearValue(v)
		if err != nil {
			t.Fatalf("parseYearValue(%q): %v", v, err)
		}
		yf.Include = append(yf.Include, r)
	}
	return yf
}

func TestParseYearValue(t *testing.T) {
	tests := []struct {
		in      string
		want    yearRange
		wantErr bool
	}{
		{in: "1975", want: yearRange{1975, 1975}},
		{in: " 1975 ", want: yearRange{1975, 1975}},
		{in: "1970-1980", want: yearRange{1970, 1980}},
		{in: "1980-1970", want: yearRange{1970, 1980}}, // auto-swap, as in v2.3.0
		{in: "1970 - 1980", want: yearRange{1970, 1980}},
		{in: "nineteen", wantErr: true},
		{in: "1970-", wantErr: true},
		{in: "1970-1980-1990", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseYearValue(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseYearValue(%q) = %v, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseYearValue(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseYearValue(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestYearFilterMatches(t *testing.T) {
	tests := []struct {
		name string
		yf   YearFilter
		year int
		want bool
	}{
		{"unconstrained", YearFilter{}, 1975, true},
		{"include single hit", YearFilter{Include: []yearRange{{1975, 1975}}}, 1975, true},
		{"include single miss", YearFilter{Include: []yearRange{{1975, 1975}}}, 1976, false},
		{"include range hit", YearFilter{Include: []yearRange{{1970, 1980}}}, 1975, true},
		{"include is OR", YearFilter{Include: []yearRange{{1959, 1959}, {1970, 1979}}}, 1975, true},
		{"exclude hit", YearFilter{Exclude: []yearRange{{1970, 1979}}}, 1975, false},
		{"exclude miss", YearFilter{Exclude: []yearRange{{1970, 1979}}}, 1985, true},
		{"exclusion beats inclusion", YearFilter{Include: []yearRange{{1970, 1980}}, Exclude: []yearRange{{1975, 1975}}}, 1975, false},
		{"unknown year fails an inclusion", YearFilter{Include: []yearRange{{1970, 1980}}}, 0, false},
		{"unknown year survives an exclusion", YearFilter{Exclude: []yearRange{{1970, 1980}}}, 0, true},
		{"unknown year with no year filter", YearFilter{}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.yf.matches(tt.year); got != tt.want {
				t.Errorf("matches(%d) = %v, want %v", tt.year, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test . -run 'TestParseYearValue|TestYearFilterMatches' -v`
Expected: compile failure — `undefined: parseYearValue`, `undefined: YearFilter`.

- [ ] **Step 3: Write the implementation**

Add `"errors"` to `filter.go`'s import block, then add below `FieldFilter`:

```go
// yearRange is an inclusive span of years. A single year is a range of one.
type yearRange struct{ start, end int }

func (r yearRange) contains(year int) bool { return year >= r.start && year <= r.end }

// YearFilter is FieldFilter's shape over parsed ranges, because --year
// compares numerically rather than by substring. --decade appends to it: they
// are two spellings of one field, so --year 1959 --decade 70s means "1959 or
// the 70s" rather than the empty intersection of two AND-ed fields.
type YearFilter struct {
	Include []yearRange
	Exclude []yearRange
}

// matches reports whether an album's year passes. A zero year means Discogs
// gave none: it falls in no range, so an inclusion never accepts it and an
// exclusion never rejects it.
func (yf YearFilter) matches(year int) bool {
	if year == 0 {
		return len(yf.Include) == 0
	}
	for _, r := range yf.Exclude {
		if r.contains(year) {
			return false
		}
	}
	if len(yf.Include) == 0 {
		return true
	}
	for _, r := range yf.Include {
		if r.contains(year) {
			return true
		}
	}
	return false
}

// errBadYearFormat keeps the wording v2.3.0 shipped, because it is what users
// have already seen and scripted against.
var errBadYearFormat = errors.New("invalid year format. Use --year 1975 or --year 1970-1980")

// parseYearValue parses one --year value: a single year, or a "start-end"
// range whose ends are swapped when given backwards.
func parseYearValue(s string) (yearRange, error) {
	s = strings.TrimSpace(s)

	if strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		if len(parts) != 2 {
			return yearRange{}, errBadYearFormat
		}
		start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return yearRange{}, errBadYearFormat
		}
		if start > end {
			start, end = end, start
		}
		return yearRange{start, end}, nil
	}

	year, err := strconv.Atoi(s)
	if err != nil {
		return yearRange{}, errBadYearFormat
	}
	return yearRange{year, year}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -run 'TestParseYearValue|TestYearFilterMatches' -v`
Expected: PASS.

- [ ] **Step 5: Run the whole suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add filter.go filter_test.go
git commit -m "feat: add YearFilter over parsed year ranges

Parses --year values once at flag time instead of re-parsing on every
album, and gives the year field the same include/exclude shape as
FieldFilter. A zero year is in no range, so an inclusion never accepts
it and an exclusion never rejects it.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 3: `parseDecadeValue`

The decade grammar, including the refusal of the three genuinely ambiguous inputs. Standalone; the flag arrives in Task 5.

**Files:**
- Modify: `filter.go`
- Test: `filter_test.go`

**Interfaces:**
- Consumes: `yearRange` from Task 2.
- Produces: `func parseDecadeValue(s string) (yearRange, error)`

- [ ] **Step 1: Write the failing test**

Append to `filter_test.go`:

```go
func TestParseDecadeValue(t *testing.T) {
	tests := []struct {
		in   string
		want yearRange
	}{
		{"70s", yearRange{1970, 1979}},
		{"1970s", yearRange{1970, 1979}},
		{"1970", yearRange{1970, 1979}},
		{"2020s", yearRange{2020, 2029}},
		{"30s", yearRange{1930, 1939}},
		{"90s", yearRange{1990, 1999}},
		{"70S", yearRange{1970, 1979}}, // case-insensitive
		{" 70s ", yearRange{1970, 1979}},
		{"1975", yearRange{1970, 1979}}, // any year names its decade
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseDecadeValue(tt.in)
			if err != nil {
				t.Fatalf("parseDecadeValue(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseDecadeValue(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// The three two-digit decades that could mean either century are refused
// outright, with a message naming both spellings. A rule that guesses would
// either put 2020s permanently out of reach or change what --decade 30s means
// in 2030.
func TestParseDecadeValueRejectsAmbiguous(t *testing.T) {
	for in, both := range map[string][2]string{
		"00s": {"1900s", "2000s"},
		"10s": {"1910s", "2010s"},
		"20s": {"1920s", "2020s"},
		"25s": {"1920s", "2020s"},
	} {
		_, err := parseDecadeValue(in)
		if err == nil {
			t.Errorf("parseDecadeValue(%q) succeeded, want an ambiguity error", in)
			continue
		}
		if !strings.Contains(err.Error(), "ambiguous") {
			t.Errorf("parseDecadeValue(%q) error = %q, want it to say ambiguous", in, err)
		}
		for _, spelling := range both {
			if !strings.Contains(err.Error(), spelling) {
				t.Errorf("parseDecadeValue(%q) error = %q, want it to name %s", in, err, spelling)
			}
		}
	}
}

func TestParseDecadeValueRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "s", "7s", "197s", "abc", "-5", "19700s"} {
		if got, err := parseDecadeValue(in); err == nil {
			t.Errorf("parseDecadeValue(%q) = %v, want an error", in, got)
		}
	}
}
```

`filter_test.go` needs `"strings"` in its import block for this task.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run TestParseDecadeValue -v`
Expected: compile failure — `undefined: parseDecadeValue`.

- [ ] **Step 3: Write the implementation**

Add to `filter.go`, below `parseYearValue`:

```go
// parseDecadeValue parses one --decade value into the ten years it names.
// Accepted: 1970s, 1970, any year within a decade, and the two-digit forms
// 30s through 90s -- unambiguous because there are no 2030s pressings yet.
// Refused: 00s, 10s and 20s, which could name either century.
//
// The alternatives were "two digits always mean 19xx", which puts the 2020s
// permanently out of reach of a two-digit value, and "the most recent decade
// that has begun", which silently changes what --decade 30s means in 2030 and
// forces every test onto a fixed clock. Refusing the three genuinely
// ambiguous inputs is the only rule that is both stable and honest.
func parseDecadeValue(s string) (yearRange, error) {
	badFormat := fmt.Errorf("invalid decade %q. Use --decade 70s or --decade 1970s", s)

	v := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), "s")
	if len(v) != 2 && len(v) != 4 {
		return yearRange{}, badFormat
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return yearRange{}, badFormat
	}

	if len(v) == 2 {
		d := n - n%10
		if d < 30 {
			return yearRange{}, fmt.Errorf("ambiguous decade %q: write 19%02ds or 20%02ds", s, d, d)
		}
		n = 1900 + d
	}

	start := n - n%10
	return yearRange{start, start + 9}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -run TestParseDecadeValue -v`
Expected: PASS, all three tests.

- [ ] **Step 5: Run the whole suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add filter.go filter_test.go
git commit -m "feat: parse --decade values, refusing the ambiguous ones

70s through 90s are unambiguous; 00s, 10s and 20s are not, and are
refused with a message naming both spellings rather than guessed at. A
guessing rule would either put the 2020s permanently out of reach of a
two-digit value or change what 30s means in 2030.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 4: The field table, and `Filter`'s new shape

The breaking change, done in one commit because every construction site must move together or the package will not compile. **Behavior is unchanged** — the CLI still registers exactly the five flags it does today, each still taking one value. Only the internal shape moves.

**Files:**
- Modify: `filter.go` (rewrite `Filter`, `Apply`, `matches`; delete `matchesQuery`, `matchesYear`, `matchesGenre`, `matchesFormats`, `matchesString`, `ParseYearFilter`)
- Modify: `cli.go:79-92` (`filterFlags.Filter()` — interim version, five flags still)
- Test: `filter_test.go`, `favorites_test.go:183`, `favorites_test.go:290`, `discogs_test.go:331`

**Interfaces:**
- Consumes: `FieldFilter`, `YearFilter`, `parseYearValue` from Tasks 1–2.
- Produces:
  - `type filterField struct { name, help string; albumValue func(Album) []string; part func(*Filter) *FieldFilter }`
  - `var filterFields []filterField` — order is `query, artist, title, genre, label, format`
  - `const queryField = 0` — the index of the query entry
  - `Filter{ Query, Artist, Title, Genre, Label, Format FieldFilter; Year YearFilter; ReleaseID int }`
  - `func (f Filter) any() bool`

- [ ] **Step 1: Write the failing test**

Append to `filter_test.go`:

```go
// queryField is an index rather than a lookup, so a test pins the assumption.
func TestQueryIsTheFirstFilterField(t *testing.T) {
	if filterFields[queryField].name != "query" {
		t.Fatalf("filterFields[queryField].name = %q, want %q",
			filterFields[queryField].name, "query")
	}
}

// Every table entry must read something off an album and point at a distinct
// part of a Filter. A copy-pasted entry that forgets to change one of the two
// closures is the failure this catches.
func TestFilterFieldsAreWiredDistinctly(t *testing.T) {
	var f Filter
	seen := map[*FieldFilter]string{}
	for _, field := range filterFields {
		p := field.part(&f)
		if p == nil {
			t.Errorf("%s: part() returned nil", field.name)
			continue
		}
		if other, dup := seen[p]; dup {
			t.Errorf("%s and %s point at the same FieldFilter", field.name, other)
		}
		seen[p] = field.name
		if field.albumValue == nil {
			t.Errorf("%s: albumValue is nil", field.name)
		}
		if field.help == "" {
			t.Errorf("%s: help is empty", field.name)
		}
	}
	if len(seen) != 6 {
		t.Errorf("wired %d fields, want 6 (query, artist, title, genre, label, format)", len(seen))
	}
}

func TestFilterFieldsReadTheRightAlbumValues(t *testing.T) {
	album := Album{
		Artist: "Miles Davis",
		Title:  "Kind of Blue",
		Label:  "Columbia",
		Genres: []string{"Jazz", "Bebop"},
		Formats: []string{"Vinyl", "LP", "Blue Translucent"},
	}
	want := map[string][]string{
		"query":  {"Miles Davis - Kind of Blue"},
		"artist": {"Miles Davis"},
		"title":  {"Kind of Blue"},
		"label":  {"Columbia"},
		"genre":  {"Jazz", "Bebop"},
		"format": {"Vinyl", "LP", "Blue Translucent"},
	}

	for _, field := range filterFields {
		got := field.albumValue(album)
		exp := want[field.name]
		if len(got) != len(exp) {
			t.Errorf("%s: albumValue = %q, want %q", field.name, got, exp)
			continue
		}
		for i := range got {
			if got[i] != exp[i] {
				t.Errorf("%s: albumValue = %q, want %q", field.name, got, exp)
				break
			}
		}
	}
}

// The grammar in one test: values within a field OR, different fields AND,
// and an exclusion removes a match outright.
func TestFilterGrammar(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959, Label: "Columbia", Genres: []string{"Jazz"}},
		{Artist: "Herbie Hancock", Title: "Head Hunters", Year: 1973, Label: "Columbia", Genres: []string{"Jazz", "Funk"}},
		{Artist: "Parliament", Title: "Mothership Connection", Year: 1975, Label: "Casablanca", Genres: []string{"Funk"}},
		{Artist: "Black Sabbath", Title: "Paranoid", Year: 1970, Label: "Vertigo", Genres: []string{"Rock"}},
		{Artist: "Unknown", Title: "Untitled"}, // no year, no label, no genres
	}

	tests := []struct {
		name   string
		filter Filter
		want   []string // artists, in order
	}{
		{
			name:   "one value behaves as it always has",
			filter: Filter{Genre: include("jazz")},
			want:   []string{"Miles Davis", "Herbie Hancock"},
		},
		{
			name:   "values within a field OR",
			filter: Filter{Genre: include("jazz", "funk")},
			want:   []string{"Miles Davis", "Herbie Hancock", "Parliament"},
		},
		{
			name:   "different fields AND",
			filter: Filter{Genre: include("jazz", "funk"), Label: include("columbia")},
			want:   []string{"Miles Davis", "Herbie Hancock"},
		},
		{
			name:   "an exclusion removes matches",
			filter: Filter{Genre: exclude("rock")},
			want:   []string{"Miles Davis", "Herbie Hancock", "Parliament", "Unknown"},
		},
		{
			name:   "an exclusion drops an album that also matches something else",
			filter: Filter{Genre: include("jazz"), Label: exclude("columbia")},
			want:   nil,
		},
		{
			name:   "exclusion beats inclusion",
			filter: Filter{Genre: FieldFilter{Include: []string{"jazz"}, Exclude: []string{"jazz"}}},
			want:   nil,
		},
		{
			name:   "an empty field is excluded by nothing",
			filter: Filter{Label: exclude("columbia")},
			want:   []string{"Parliament", "Black Sabbath", "Unknown"},
		},
		{
			name:   "an unknown year is excluded by nothing",
			filter: Filter{Year: YearFilter{Exclude: []yearRange{{1959, 1959}}}},
			want:   []string{"Herbie Hancock", "Parliament", "Black Sabbath", "Unknown"},
		},
		{
			name:   "year and decade ranges OR together",
			filter: Filter{Year: YearFilter{Include: []yearRange{{1959, 1959}, {1970, 1979}}}},
			want:   []string{"Miles Davis", "Herbie Hancock", "Parliament", "Black Sabbath"},
		},
		{
			name:   "artist and title are separate fields",
			filter: Filter{Artist: include("miles")},
			want:   []string{"Miles Davis"},
		},
		{
			name:   "title does not match the artist",
			filter: Filter{Title: include("miles")},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Apply(albums)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d albums %v, want %d %v", len(got), artistsOf(got), len(tt.want), tt.want)
			}
			for i, a := range got {
				if a.Artist != tt.want[i] {
					t.Errorf("album %d = %q, want %q", i, a.Artist, tt.want[i])
				}
			}
		})
	}
}

func artistsOf(albums []Album) []string {
	out := make([]string, len(albums))
	for i, a := range albums {
		out[i] = a.Artist
	}
	return out
}

// An unset filter returns the input untouched, which is what keeps `list`
// from copying the whole collection for nothing.
func TestFilterAnyReportsWhetherAnythingIsSet(t *testing.T) {
	if (Filter{}).any() {
		t.Error("empty Filter.any() = true, want false")
	}
	for name, f := range map[string]Filter{
		"query include":   {Query: include("miles")},
		"query exclude":   {Query: exclude("bootleg")},
		"artist":          {Artist: include("miles")},
		"title":           {Title: include("blue")},
		"genre":           {Genre: include("jazz")},
		"label":           {Label: include("columbia")},
		"format":          {Format: include("vinyl")},
		"year include":    {Year: YearFilter{Include: []yearRange{{1975, 1975}}}},
		"year exclude":    {Year: YearFilter{Exclude: []yearRange{{1975, 1975}}}},
		"release id":      {ReleaseID: 1839278},
	} {
		if !f.any() {
			t.Errorf("%s: any() = false, want true", name)
		}
	}
}
```

Now update the existing literals. In `filter_test.go`:

- `TestFilterByYear` — replace the body of the subtest loop's filter construction:
  ```go
  var f Filter
  if tt.yearFilter != "" {
      f = Filter{Year: years(t, tt.yearFilter)}
  }
  ```
- `TestFilterByGenre`: `Filter{Genre: "jazz"}` → `Filter{Genre: include("jazz")}`
- `TestFilterCombined`: `Filter{Year: "1970", Genre: "jazz"}` → `Filter{Year: years(t, "1970"), Genre: include("jazz")}`
- Every `Filter{Query: "X"}` → `Filter{Query: include("X")}`, **except** `Filter{Query: ""}` (line 121), which becomes `Filter{}` — an empty query has always meant "no query filter", and that is the faithful translation.
- `Filter{Query: "miles", Year: "1959"}` → `Filter{Query: include("miles"), Year: years(t, "1959")}`
- `Filter{ReleaseID: ...}` and `Filter{}` are unchanged.

In `favorites_test.go`, lines 183 and 290: `Filter{Year: "1959"}` → `Filter{Year: years(t, "1959")}`.

In `discogs_test.go`, line 331: `(Filter{Format: "blue"})` → `(Filter{Format: include("blue")})`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./...`
Expected: compile failure — `undefined: filterFields`, `unknown field Artist in struct literal`.

- [ ] **Step 3: Rewrite `filter.go`'s `Filter`, table, and matching**

Replace the existing `Filter` type, `Apply`, `matches`, `matchesQuery`, `matchesYear`, `matchesGenre`, `matchesFormats`, `matchesString` and `ParseYearFilter` with:

```go
// filterField describes one substring-matched filter: the flag name it is
// spelled with, its line of help, what it reads off an album, and where its
// parsed values land in a Filter. Flag registration, help generation and
// matching all loop over the table, so a new substring filter is one entry
// here rather than four edits across three files.
//
// --year is deliberately not in the table: it parses its values and compares
// them numerically, and two flag names (--year and --decade) feed it, so
// forcing it into this shape would cost more than the duplication saves.
type filterField struct {
	name       string
	help       string
	albumValue func(Album) []string
	part       func(*Filter) *FieldFilter
}

// queryField is the index of the query entry below. Query is special twice
// over: it is the one field that satisfies favorite's "requires a query"
// rule, and the one whose inclusions do not count as narrowing.
// TestQueryIsTheFirstFilterField pins this.
const queryField = 0

var filterFields = []filterField{
	{
		name:       "query",
		help:       `Filter by "Artist - Title" (case-insensitive substring)`,
		albumValue: func(a Album) []string { return []string{a.Key()} },
		part:       func(f *Filter) *FieldFilter { return &f.Query },
	},
	{
		name:       "artist",
		help:       "Filter by artist",
		albumValue: func(a Album) []string { return []string{a.Artist} },
		part:       func(f *Filter) *FieldFilter { return &f.Artist },
	},
	{
		name:       "title",
		help:       "Filter by title",
		albumValue: func(a Album) []string { return []string{a.Title} },
		part:       func(f *Filter) *FieldFilter { return &f.Title },
	},
	{
		name:       "genre",
		help:       "Filter by genre",
		albumValue: func(a Album) []string { return a.Genres },
		part:       func(f *Filter) *FieldFilter { return &f.Genre },
	},
	{
		name:       "label",
		help:       "Filter by label",
		albumValue: func(a Album) []string { return []string{a.Label} },
		part:       func(f *Filter) *FieldFilter { return &f.Label },
	},
	{
		name: "format",
		// Format matches any entry of Album.Formats, which includes the
		// format name, its descriptions, and its free text -- the last
		// being where Discogs records a pressing's colour.
		help:       "Filter by format or colour",
		albumValue: func(a Album) []string { return a.Formats },
		part:       func(f *Filter) *FieldFilter { return &f.Format },
	},
}

// Filter narrows a collection. Values within a field OR together, different
// fields AND together, and any exclusion removes a match outright.
type Filter struct {
	Query, Artist, Title, Genre, Label, Format FieldFilter
	Year                                       YearFilter
	// ReleaseID selects one exact record. Zero means unset. Unlike the
	// fields above it identifies rather than narrows, which is why it is
	// compared whole, needs no query alongside it, and takes neither
	// several values nor an exclusion.
	ReleaseID int
}

// Apply returns the albums matching the filter. An unset filter returns the
// input untouched rather than copying it.
func (f Filter) Apply(albums []Album) []Album {
	if !f.any() {
		return albums
	}

	var filtered []Album
	for _, album := range albums {
		if f.matches(album) {
			filtered = append(filtered, album)
		}
	}
	return filtered
}

// any reports whether the filter constrains anything at all.
func (f Filter) any() bool {
	if f.ReleaseID != 0 || len(f.Year.Include) > 0 || len(f.Year.Exclude) > 0 {
		return true
	}
	for _, field := range filterFields {
		p := field.part(&f)
		if len(p.Include) > 0 || len(p.Exclude) > 0 {
			return true
		}
	}
	return false
}

func (f Filter) matches(album Album) bool {
	if f.ReleaseID != 0 && album.ReleaseID != f.ReleaseID {
		return false
	}
	for _, field := range filterFields {
		if !field.part(&f).matches(field.albumValue(album)) {
			return false
		}
	}
	return f.Year.matches(album.Year)
}
```

- [ ] **Step 4: Update `filterFlags.Filter()` in `cli.go` to keep the build green**

Still five flags, still one value each — the new flags arrive in Task 5. Replace the body of `func (ff *filterFlags) Filter() (Filter, error)`:

```go
// Filter builds a Filter from the parsed flags, validating the year format.
func (ff *filterFlags) Filter() (Filter, error) {
	f := Filter{ReleaseID: *ff.releaseID}

	if *ff.year != "" {
		r, err := parseYearValue(*ff.year)
		if err != nil {
			return Filter{}, err
		}
		f.Year.Include = append(f.Year.Include, r)
	}
	if *ff.genre != "" {
		f.Genre.Include = []string{*ff.genre}
	}
	if *ff.label != "" {
		f.Label.Include = []string{*ff.label}
	}
	if *ff.format != "" {
		f.Format.Include = []string{*ff.format}
	}
	return f, nil
}
```

- [ ] **Step 5: Fix `favoriteByQuery` and `unfavoriteByQuery`'s query assignment**

`favorites.go:144` and `favorites.go:184` both do `filter.Query = query`, which no longer compiles. Change each to:

```go
	if query != "" {
		filter.Query.Include = append(filter.Query.Include, query)
	}
```

(Task 7 retires the parameter entirely; this keeps the build green in between.)

- [ ] **Step 6: Run the whole suite**

Run: `go test ./...`
Expected: PASS. Every pre-existing test passes with only its struct literals rewritten — that is the proof that behavior did not change.

- [ ] **Step 7: Verify no behavior drift at the command line**

```bash
go build -o /tmp/df-t6 . && /tmp/df-t6 list --genre jazz | head -5
```
Expected: the same output as before this task. (Skip if no local collection exists; `go test ./...` is the gate.)

- [ ] **Step 8: Commit**

```bash
git add filter.go cli.go favorites.go filter_test.go favorites_test.go discogs_test.go
git commit -m "refactor: give Filter include/exclude fields and a field table

Every narrowing field becomes a FieldFilter and the six substring
filters move into one table that registration, help and matching all
loop over. No behavior change: the CLI still registers five flags taking
one value each, and every pre-existing test passes with only its struct
literals rewritten.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 5: The flags — repetition, exclusion, `--artist`, `--title`, `--decade`, `--query`

Where the feature becomes visible.

**Files:**
- Modify: `cli.go` (`filterFlags`, `addFilterFlags`, `Filter()`, `anyNarrowing()`, new `hasQuery()`, `filterFlagHelp`)
- Test: `cli_test.go`

**Interfaces:**
- Consumes: `filterFields`, `queryField`, `parseYearValue`, `parseDecadeValue`.
- Produces:
  - `filterFlags` holding `include, exclude []*arrayFlags` indexed by `filterFields` position, plus `year, noYear, decade, noDecade arrayFlags` and `releaseID *int`
  - `func (ff *filterFlags) hasQuery() bool`
  - `func (ff *filterFlags) queryValues() []string`
  - `func nonEmpty(vals []string) []string`
  - `func parseYearValues(years, decades []string) ([]yearRange, error)`
  - `filterFlagHelp` generated from the table

- [ ] **Step 1: Write the failing test**

Append to `cli_test.go`:

```go
func TestFilterFlagsRepeatAndOR(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{"--genre", "jazz", "--genre", "funk"}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(filter.Genre.Include) != 2 ||
		filter.Genre.Include[0] != "jazz" || filter.Genre.Include[1] != "funk" {
		t.Errorf("Genre.Include = %q, want [jazz funk]", filter.Genre.Include)
	}
}

func TestFilterFlagsExcludeTwins(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{
		"--exclude-query", "bootleg",
		"--exclude-artist", "davis",
		"--exclude-title", "live",
		"--exclude-genre", "rock",
		"--exclude-label", "columbia",
		"--exclude-format", "cd",
	}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	for name, got := range map[string][]string{
		"query":  filter.Query.Exclude,
		"artist": filter.Artist.Exclude,
		"title":  filter.Title.Exclude,
		"genre":  filter.Genre.Exclude,
		"label":  filter.Label.Exclude,
		"format": filter.Format.Exclude,
	} {
		if len(got) != 1 {
			t.Errorf("%s: Exclude = %q, want one value", name, got)
		}
	}
}

func TestFilterFlagsNewNarrowingFields(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{"--query", "kind of", "--artist", "miles", "--title", "blue"}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(filter.Query.Include) != 1 || filter.Query.Include[0] != "kind of" {
		t.Errorf("Query.Include = %q, want [kind of]", filter.Query.Include)
	}
	if len(filter.Artist.Include) != 1 || filter.Artist.Include[0] != "miles" {
		t.Errorf("Artist.Include = %q, want [miles]", filter.Artist.Include)
	}
	if len(filter.Title.Include) != 1 || filter.Title.Include[0] != "blue" {
		t.Errorf("Title.Include = %q, want [blue]", filter.Title.Include)
	}
}

// --year and --decade are two spellings of one field, so they OR rather than
// AND. The naive reading -- two separate fields intersected -- would make
// this combination return nothing at all.
func TestYearAndDecadeFeedOneConstraint(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "1959", "--decade", "70s"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(filter.Year.Include) != 2 {
		t.Fatalf("Year.Include = %v, want two ranges", filter.Year.Include)
	}
	albums := []Album{
		{Artist: "A", Year: 1959},
		{Artist: "B", Year: 1975},
		{Artist: "C", Year: 1985},
	}
	if got := filter.Apply(albums); len(got) != 2 {
		t.Errorf("matched %d albums, want 2 (1959 and 1975)", len(got))
	}
}

func TestExcludeYearAndDecadeFeedOneExclusion(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{"--exclude-year", "1959", "--exclude-decade", "70s"}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(filter.Year.Exclude) != 2 {
		t.Errorf("Year.Exclude = %v, want two ranges", filter.Year.Exclude)
	}
	if len(filter.Year.Include) != 0 {
		t.Errorf("Year.Include = %v, want none", filter.Year.Include)
	}
}

func TestFilterFlagsRejectsAmbiguousDecade(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--decade", "20s"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	_, err := ff.Filter()
	if err == nil {
		t.Fatal("expected an error for --decade 20s")
	}
	if !strings.Contains(err.Error(), "1920s") || !strings.Contains(err.Error(), "2020s") {
		t.Errorf("error = %q, want it to name both spellings", err)
	}
}

// `--genre "$GENRE"` with an unset variable has always meant "no genre
// filter", and must keep meaning that. It also matters for exclusions: every
// string contains "", so an empty exclusion reaching the matcher would
// exclude the whole collection.
func TestEmptyFilterValuesAreDropped(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{"--genre", "", "--exclude-genre", "", "--year", "", "--decade", ""}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if filter.any() {
		t.Errorf("filter = %+v, want nothing set", filter)
	}

	albums := []Album{{Artist: "A", Genres: []string{"Jazz"}}, {Artist: "B"}}
	if got := filter.Apply(albums); len(got) != 2 {
		t.Errorf("matched %d albums, want all 2", len(got))
	}
}

func TestHasQueryIgnoresExclusions(t *testing.T) {
	fs, _ := newFlagSet("favorite")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--exclude-query", "bootleg"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if ff.hasQuery() {
		t.Error("hasQuery() = true for --exclude-query; an exclusion says which record is NOT meant")
	}
	if !ff.anyNarrowing() {
		t.Error("anyNarrowing() = false for --exclude-query, want true")
	}
}

func TestHasQueryAndNarrowing(t *testing.T) {
	fs, _ := newFlagSet("favorite")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--query", "miles"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if !ff.hasQuery() {
		t.Error("hasQuery() = false for --query, want true")
	}
	if ff.anyNarrowing() {
		t.Error("anyNarrowing() = true for --query alone; a query is not a narrowing filter")
	}
}

// pick and list have never had a free-text search: they reject positional
// arguments, and --query did not exist. This is the acceptance criterion for
// that gap.
func TestParseSelectionAcceptsQueryAndTheNewFilters(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		args := []string{
			"--query", "miles",
			"--artist", "davis",
			"--title", "blue",
			"--decade", "70s",
			"--exclude-genre", "rock",
		}
		cfg, err := parseSelection(name, args)
		if err != nil {
			t.Fatalf("parseSelection(%s): %v", name, err)
		}
		if len(cfg.filter.Query.Include) != 1 || cfg.filter.Query.Include[0] != "miles" {
			t.Errorf("%s: Query.Include = %q, want [miles]", name, cfg.filter.Query.Include)
		}
		if len(cfg.filter.Artist.Include) != 1 || len(cfg.filter.Title.Include) != 1 {
			t.Errorf("%s: artist/title not parsed: %+v", name, cfg.filter)
		}
		if len(cfg.filter.Year.Include) != 1 || cfg.filter.Year.Include[0] != (yearRange{1970, 1979}) {
			t.Errorf("%s: Year.Include = %v, want [{1970 1979}]", name, cfg.filter.Year.Include)
		}
		if len(cfg.filter.Genre.Exclude) != 1 || cfg.filter.Genre.Exclude[0] != "rock" {
			t.Errorf("%s: Genre.Exclude = %q, want [rock]", name, cfg.filter.Genre.Exclude)
		}
	}
}

func TestEveryNarrowingFlagCountsAsNarrowing(t *testing.T) {
	for _, args := range [][]string{
		{"--artist", "miles"},
		{"--title", "blue"},
		{"--genre", "jazz"},
		{"--label", "columbia"},
		{"--format", "vinyl"},
		{"--year", "1959"},
		{"--decade", "70s"},
		{"--exclude-artist", "miles"},
		{"--exclude-genre", "rock"},
		{"--exclude-year", "1959"},
		{"--exclude-decade", "70s"},
	} {
		fs, _ := newFlagSet("favorite")
		ff := addFilterFlags(fs)
		if _, err := parseInterspersed(fs, args); err != nil {
			t.Fatalf("parseInterspersed(%v): %v", args, err)
		}
		if !ff.anyNarrowing() {
			t.Errorf("anyNarrowing() = false for %v, want true", args)
		}
	}
}
```

Update `TestFilterFlagsBuildsFilter` (`cli_test.go:92`), whose assertion compares scalars:

```go
	if len(filter.Year.Include) != 1 || filter.Year.Include[0] != (yearRange{1970, 1980}) {
		t.Errorf("Year.Include = %v, want [{1970 1980}]", filter.Year.Include)
	}
	if len(filter.Genre.Include) != 1 || filter.Genre.Include[0] != "jazz" {
		t.Errorf("Genre.Include = %q, want [jazz]", filter.Genre.Include)
	}
```

And `TestFavoriteAcceptsReleaseIDWithNarrowingFilters` (`cli_test.go:672`):

```go
	if cfg.filter.ReleaseID != 1839278 || len(cfg.filter.Year.Include) != 1 {
		t.Errorf("filter = %+v, want both the ID and the year", cfg.filter)
	}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestFilterFlags|TestYearAndDecade|TestExcludeYear|TestEmptyFilter|TestHasQuery|TestEveryNarrowing|TestParseSelectionAccepts' -v`
Expected: failures — `flag provided but not defined: -exclude-genre`, `ff.hasQuery undefined`.

- [ ] **Step 3: Rewrite the flag plumbing in `cli.go`**

Replace `filterFlags`, `addFilterFlags`, `Filter()`, `anyNarrowing()` and `identifies()`:

```go
// filterFlags holds the filter flags shared by pick, list, favorite and
// unfavorite. Registering them in one place keeps their names and help text
// from drifting apart between commands.
//
// Every narrowing filter is repeatable and has an --exclude-NAME twin;
// include and exclude are indexed by position in filterFields. --release-id
// is the exception, because it identifies one record rather than narrowing a
// query.
type filterFlags struct {
	include []*arrayFlags
	exclude []*arrayFlags
	// year and decade are two spellings of one constraint, kept apart
	// only long enough to parse them differently.
	year      arrayFlags
	noYear    arrayFlags
	decade    arrayFlags
	noDecade  arrayFlags
	releaseID *int
}

func addFilterFlags(fs *flag.FlagSet) *filterFlags {
	ff := &filterFlags{
		include: make([]*arrayFlags, len(filterFields)),
		exclude: make([]*arrayFlags, len(filterFields)),
	}
	for i, field := range filterFields {
		inc, exc := new(arrayFlags), new(arrayFlags)
		fs.Var(inc, field.name, field.help+" (repeatable)")
		fs.Var(exc, "exclude-"+field.name, "Exclude matches of "+field.name+" (repeatable)")
		ff.include[i], ff.exclude[i] = inc, exc
	}
	fs.Var(&ff.year, "year", "Filter by year or year range (e.g., 1975 or 1970-1980)")
	fs.Var(&ff.noYear, "exclude-year", "Exclude a year or year range (repeatable)")
	fs.Var(&ff.decade, "decade", "Filter by decade (e.g., 70s or 1970s); adds to --year")
	fs.Var(&ff.noDecade, "exclude-decade", "Exclude a decade (repeatable)")
	ff.releaseID = fs.Int("release-id", 0, "Select one exact record by its Discogs release ID")
	return ff
}

// Filter builds a Filter from the parsed flags, validating year and decade
// values.
func (ff *filterFlags) Filter() (Filter, error) {
	f := Filter{ReleaseID: *ff.releaseID}
	for i, field := range filterFields {
		p := field.part(&f)
		p.Include = nonEmpty(*ff.include[i])
		p.Exclude = nonEmpty(*ff.exclude[i])
	}

	var err error
	if f.Year.Include, err = parseYearValues(ff.year, ff.decade); err != nil {
		return Filter{}, err
	}
	if f.Year.Exclude, err = parseYearValues(ff.noYear, ff.noDecade); err != nil {
		return Filter{}, err
	}
	return f, nil
}

// nonEmpty drops empty values, so `--genre "$GENRE"` with an unset variable
// keeps meaning "no genre filter" as it always has. It matters more for
// exclusions: every string contains "", so an empty --exclude-genre reaching
// the matcher would exclude the entire collection.
func nonEmpty(vals []string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// parseYearValues turns --year and --decade values into one list of ranges.
// They feed a single constraint on purpose: --year 1959 --decade 70s means
// "1959 or the 70s", not the empty intersection two AND-ed fields would give.
func parseYearValues(years, decades []string) ([]yearRange, error) {
	var out []yearRange
	for _, v := range years {
		if v == "" {
			continue
		}
		r, err := parseYearValue(v)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	for _, v := range decades {
		if v == "" {
			continue
		}
		r, err := parseDecadeValue(v)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// anyNarrowing reports whether a filter that only *refines* a query was set.
// Those cannot stand alone: --year 1959 does not say which record is meant.
//
// Two deliberate exclusions from the count. --release-id identifies one exact
// record and needs nothing beside it. A --query *inclusion* is itself a
// query, so it is reported by hasQuery instead -- but an --exclude-query only
// says which record is not meant, so it narrows like any other exclusion.
func (ff *filterFlags) anyNarrowing() bool {
	for i := range filterFields {
		if i != queryField && len(*ff.include[i]) > 0 {
			return true
		}
		if len(*ff.exclude[i]) > 0 {
			return true
		}
	}
	return len(ff.year) > 0 || len(ff.noYear) > 0 ||
		len(ff.decade) > 0 || len(ff.noDecade) > 0
}

// hasQuery reports whether --query named something to look for, which is what
// lets it satisfy favorite's "requires a query" rule.
func (ff *filterFlags) hasQuery() bool {
	return len(nonEmpty(*ff.include[queryField])) > 0
}

// queryValues returns the --query values, empty ones dropped.
func (ff *filterFlags) queryValues() []string {
	return nonEmpty(*ff.include[queryField])
}

// identifies reports whether the flags name one exact record on their own.
func (ff *filterFlags) identifies() bool {
	return *ff.releaseID != 0
}
```

- [ ] **Step 4: Generate `filterFlagHelp` from the table**

Replace the `const filterFlagHelp = ...` block (`cli.go:446`) with a `var` built in the same file, above `init()`:

```go
// filterFlagHelp is the shared help block for the filter flags, generated
// from filterFields so a new filter cannot ship undocumented. The
// --exclude-NAME twins are named once by the heading rather than listed:
// sixteen near-identical lines would bury the eight that matter.
// TestFilterFlagsAreDocumented enforces both halves of that bargain.
var filterFlagHelp = buildFilterFlagHelp()

func buildFilterFlagHelp() string {
	var sb strings.Builder
	sb.WriteString("\nFilters (all repeatable; each has an --exclude-NAME twin that removes matches):\n")
	for _, field := range filterFields {
		fmt.Fprintf(&sb, "  --%-12s VALUE  %s\n", field.name, field.help)
	}
	fmt.Fprintf(&sb, "  --%-12s VALUE  %s\n", "year", "Filter by year or year range (e.g., 1975 or 1970-1980)")
	fmt.Fprintf(&sb, "  --%-12s VALUE  %s\n", "decade", "Filter by decade (e.g., 70s or 1970s); adds to --year")
	fmt.Fprintf(&sb, "  --%-12s N      %s\n", "release-id", "Select one exact record by its Discogs release ID")
	return sb.String()
}
```

The old constant began with `  --year` and no leading newline, and each usage block appends it straight after a line ending in `\n` (`cli.go:472`, `cli.go:492`, `cli.go:566`). The generated block opens with its own `\n`, which is what puts one blank line between the command's own `Flags:` list and the shared `Filters (...)` heading. Leave those concatenation points alone — no usage string needs editing in this task.

- [ ] **Step 5: Run the new tests**

Run: `go test . -run 'TestFilterFlags|TestYearAndDecade|TestExcludeYear|TestEmptyFilter|TestHasQuery|TestEveryNarrowing|TestParseSelectionAccepts' -v`
Expected: PASS.

- [ ] **Step 6: Run the whole suite and eyeball the help**

```bash
go test ./...
go run . help pick
go run . help list
```
Expected: tests PASS; the help block lists all eight narrowing filters plus `--release-id`, with exactly one blank line before "Filters (".

- [ ] **Step 7: Commit**

```bash
git add cli.go cli_test.go
git commit -m "feat: repeatable filters, --exclude twins, --artist/--title/--decade

Every narrowing filter now takes several values that OR together and has
an --exclude-NAME twin. --query becomes a real flag, so pick and list
gain the free-text search they never had. --year and --decade feed one
constraint, so --year 1959 --decade 70s means '1959 or the 70s' rather
than nothing at all.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 6: Strengthen the documentation guard

The existing guard checks a hand-written list of five names, so it cannot catch a flag added without help text — exactly the drift it was written to stop.

**Files:**
- Modify: `global_flags_test.go:129-152`

**Interfaces:**
- Consumes: `addFilterFlags`, `newFlagSet`, `commands`.
- Produces: nothing.

- [ ] **Step 1: Replace the guard with one that walks the registered flags**

Replace `TestFilterFlagsAreDocumented` in `global_flags_test.go`:

```go
// TestFilterFlagsAreDocumented is the drift guard the filter flags lacked.
// Their help text is assembled separately from the FlagSet, so a flag added
// without help would otherwise leave every usage block quietly stale -- which
// is exactly what happened when --release-id was added.
//
// It walks what addFilterFlags actually registers rather than a hand-written
// list, so a filter added in future fails here without anyone remembering to
// update the test. A flag counts as documented when it is named literally, or
// when it is the --exclude- twin of a flag that is; the block names that
// convention once instead of listing sixteen near-identical lines, so the
// sentence introducing it is required too.
func TestFilterFlagsAreDocumented(t *testing.T) {
	base, _ := newFlagSet("pick")
	global := map[string]bool{}
	base.VisitAll(func(f *flag.Flag) { global[f.Name] = true })

	fs, _ := newFlagSet("pick")
	addFilterFlags(fs)
	var names []string
	fs.VisitAll(func(f *flag.Flag) {
		if !global[f.Name] {
			names = append(names, f.Name)
		}
	})
	if len(names) == 0 {
		t.Fatal("addFilterFlags registered nothing; the guard is not testing anything")
	}

	documented := 0
	for _, c := range commands {
		// A command takes the filter flags if it documents any of them.
		if !strings.Contains(c.usage, "--year") {
			continue
		}
		documented++

		if !strings.Contains(c.usage, "--exclude-NAME twin") {
			t.Errorf("%s usage does not explain the --exclude-NAME twins", c.name)
		}
		for _, name := range names {
			if strings.Contains(c.usage, "--"+name) {
				continue
			}
			twin, isTwin := strings.CutPrefix(name, "exclude-")
			if isTwin && strings.Contains(c.usage, "--"+twin) {
				continue
			}
			t.Errorf("%s usage does not mention --%s", c.name, name)
		}
	}
	if documented == 0 {
		t.Fatal("no command documents the filter flags; the guard is not testing anything")
	}
}
```

`global_flags_test.go` needs `"flag"` in its import block.

- [ ] **Step 2: Run it**

Run: `go test . -run TestFilterFlagsAreDocumented -v`
Expected: PASS.

- [ ] **Step 3: Prove the guard actually bites**

Temporarily add a bogus flag inside `addFilterFlags` — `fs.String("bogus", "", "undocumented")` — then run the test.

Run: `go test . -run TestFilterFlagsAreDocumented -v`
Expected: FAIL with "usage does not mention --bogus". **Remove the bogus flag** and re-run; expected PASS.

- [ ] **Step 4: Run the whole suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add global_flags_test.go
git commit -m "test: guard filter-flag docs against the registered flag set

The old guard checked a hand-written list of five names, so it could not
catch a flag added without help text -- the drift it exists to stop. It
now walks what addFilterFlags registers, accepting an --exclude- twin of
a documented flag and requiring the sentence that introduces them.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 7: `favorite` / `unfavorite` query plumbing

The positional `QUERY` becomes exactly equivalent to one `--query`, and one place owns what "the query" means.

**Files:**
- Modify: `cli.go` (`parseFavorite`), `favorites.go` (`favoriteByQuery`, `unfavoriteByQuery` signatures), `main.go` (`runFavorite`, `runUnfavorite` call sites), `cli.go` (`favorite`/`unfavorite` usage blocks)
- Test: `cli_test.go`, `favorites_test.go`

**Interfaces:**
- Consumes: `hasQuery()`, `queryValues()`, `anyNarrowing()`, `identifies()`.
- Produces:
  - `func favoriteByQuery(collection []Album, filter Filter, favPath string) (FavoriteOutcome, error)` — the `query string` parameter is gone
  - `func unfavoriteByQuery(favorites []Album, filter Filter, favPath string) (UnfavoriteOutcome, error)`
  - `favoriteConfig.query` now holds a *description* (`"miles"`, or `"miles or coltrane"` for several `--query` values); empty still means "the last pick"

- [ ] **Step 1: Write the failing test**

Append to `cli_test.go`:

```go
// The positional QUERY and --query are one thing said two ways, so the filter
// carries the value either way.
func TestFavoritePositionalQueryLandsInTheFilter(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"miles"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if len(cfg.filter.Query.Include) != 1 || cfg.filter.Query.Include[0] != "miles" {
		t.Errorf("filter.Query.Include = %q, want [miles]", cfg.filter.Query.Include)
	}
	if cfg.query != "miles" {
		t.Errorf("query = %q, want %q", cfg.query, "miles")
	}
}

func TestFavoriteQueryFlagIsEquivalentToPositional(t *testing.T) {
	positional, err := parseFavorite("favorite", []string{"miles"})
	if err != nil {
		t.Fatalf("parseFavorite positional: %v", err)
	}
	flagged, err := parseFavorite("favorite", []string{"--query", "miles"})
	if err != nil {
		t.Fatalf("parseFavorite --query: %v", err)
	}
	if flagged.query != positional.query {
		t.Errorf("query = %q, want %q", flagged.query, positional.query)
	}
	if len(flagged.filter.Query.Include) != 1 || flagged.filter.Query.Include[0] != "miles" {
		t.Errorf("filter.Query.Include = %q, want [miles]", flagged.filter.Query.Include)
	}
}

// Both spellings at once is refused rather than OR-ed. The rule in the design
// would make it an OR, but on a command that mutates favorites a surprise is
// worse than a refusal.
func TestFavoriteRejectsPositionalAndQueryFlagTogether(t *testing.T) {
	for _, name := range []string{"favorite", "unfavorite"} {
		_, err := parseFavorite(name, []string{"miles", "--query", "coltrane"})
		if err == nil {
			t.Errorf("%s: expected an error when both spellings are given", name)
			continue
		}
		if !strings.Contains(err.Error(), "give the query once") {
			t.Errorf("%s error = %q, want it to say the query is given once", name, err)
		}
	}
}

func TestFavoriteQueryFlagSatisfiesTheQueryRequirement(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"--query", "miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "miles" {
		t.Errorf("query = %q, want miles", cfg.query)
	}
	if len(cfg.filter.Year.Include) != 1 {
		t.Errorf("Year.Include = %v, want one range", cfg.filter.Year.Include)
	}
}

// An exclusion says which record is NOT meant, so it cannot stand in for a
// query on a command that has to pick exactly one record.
func TestFavoriteExcludeQueryStillRequiresAQuery(t *testing.T) {
	_, err := parseFavorite("favorite", []string{"--exclude-query", "bootleg"})
	if err == nil {
		t.Fatal("expected an error for --exclude-query alone")
	}
	if !strings.Contains(err.Error(), "filters require a query") {
		t.Errorf("error = %q, want it to mention a query", err)
	}
}

func TestFavoriteSeveralQueryValuesDescribeThemselves(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"--query", "miles", "--query", "coltrane"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "miles or coltrane" {
		t.Errorf("query = %q, want %q", cfg.query, "miles or coltrane")
	}
	if len(cfg.filter.Query.Include) != 2 {
		t.Errorf("filter.Query.Include = %q, want two values", cfg.filter.Query.Include)
	}
}
```

In `favorites_test.go`, update every call to the two changed functions — the query moves into the filter:

- `favoriteByQuery(collection, "kind of", Filter{}, favPath)` → `favoriteByQuery(collection, Filter{Query: include("kind of")}, favPath)`
- `favoriteByQuery(collection, "zzzz", Filter{}, favPath)` → `favoriteByQuery(collection, Filter{Query: include("zzzz")}, favPath)`
- `favoriteByQuery(collection, "miles", Filter{}, favPath)` → `favoriteByQuery(collection, Filter{Query: include("miles")}, favPath)` (lines 127 and 261's unfavorite twin)
- `favoriteByQuery(collection, "miles", Filter{Year: years(t, "1959")}, favPath)` → `favoriteByQuery(collection, Filter{Query: include("miles"), Year: years(t, "1959")}, favPath)`
- The same transformation for all five `unfavoriteByQuery` call sites.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./...`
Expected: failures — `too many arguments in call to favoriteByQuery`, and the new parse tests failing.

- [ ] **Step 3: Update `parseFavorite` in `cli.go`**

Replace everything from `if len(rest) == 0 {` to the end of the function:

```go
	// The positional QUERY and --query are one thing said two ways. Giving
	// both would be an OR by the grammar's own rule, but on a command that
	// mutates favorites a surprise is worse than a refusal.
	if len(rest) == 1 && ff.hasQuery() {
		return favoriteConfig{}, fmt.Errorf(
			"%s: give the query once, as an argument or --query", name)
	}

	if len(rest) == 0 {
		// A release ID is a complete answer by itself, so it excuses the
		// missing query -- and carries any narrowing filters along with it.
		if ff.anyNarrowing() && !ff.identifies() && !ff.hasQuery() {
			return favoriteConfig{}, fmt.Errorf("%s: filters require a query", name)
		}
		return favoriteConfig{
			query:  strings.Join(ff.queryValues(), " or "),
			filter: filter,
			color:  color,
		}, nil
	}

	query := strings.TrimSpace(rest[0])
	if query == "" {
		return favoriteConfig{}, fmt.Errorf("%s: requires a query", name)
	}
	// The positional query is the same constraint --query would have set, so
	// it goes to the same place. cfg.query keeps only the description: an
	// empty one still means "the last pick".
	filter.Query.Include = append(filter.Query.Include, query)
	return favoriteConfig{query: query, filter: filter, color: color}, nil
}
```

Update `favoriteConfig`'s doc comment:

```go
// favoriteConfig is the parsed form of favorite and unfavorite. query is the
// human-readable description of what was asked for -- the constraint itself
// lives in filter.Query. An empty query means "the last pick".
type favoriteConfig struct {
	query  string
	filter Filter
	color  colorMode
}
```

- [ ] **Step 4: Drop the redundant parameter in `favorites.go`**

`favoriteByQuery` (line 143) and `unfavoriteByQuery` (line 183): remove the `query string` parameter and the `filter.Query` assignment added in Task 4, so each begins straight at `matches := filter.Apply(...)`. Update both doc comments to say the query is already in the filter:

```go
// favoriteByQuery is the testable core of `favorite QUERY`. The query is
// already part of filter (parseFavorite puts the positional QUERY and --query
// in the same place), so this only applies it and acts on the result.
func favoriteByQuery(collection []Album, filter Filter, favPath string) (FavoriteOutcome, error) {
	matches := filter.Apply(collection)
```

- [ ] **Step 5: Update the two call sites in `main.go`**

`runFavorite`: `favoriteByQuery(albums, cfg.query, cfg.filter, favoritesPath())` → `favoriteByQuery(albums, cfg.filter, favoritesPath())`.
`runUnfavorite`: the matching `unfavoriteByQuery(favorites, cfg.query, cfg.filter, favoritesPath())` → `unfavoriteByQuery(favorites, cfg.filter, favoritesPath())`.

The `cfg.query == "" && cfg.filter.ReleaseID == 0` last-pick checks stay exactly as they are.

- [ ] **Step 6: Update the `favorite` and `unfavorite` usage blocks in `cli.go`**

The generated help block now carries its own `Filters (...)` heading, so the
hand-written one directly above `` ` + filterFlagHelp `` (`cli.go:565`) is a
second heading for the same list. Delete that line from both blocks, and move
what it said into the prose above, after the "Two pressings of a title..."
paragraph:

```
The QUERY can also be given as --query, which is the only difference between
the two spellings. --release-id needs neither.
```

- [ ] **Step 7: Run the whole suite**

Run: `go test ./...`
Expected: PASS, including the pre-existing `TestFavoriteStillRequiresQueryForNarrowingFilters` and `TestParseFavoriteBareMeansLastPick`.

- [ ] **Step 8: Commit**

```bash
git add cli.go favorites.go main.go cli_test.go favorites_test.go
git commit -m "feat: accept --query on favorite and unfavorite

The positional QUERY and --query are one thing said two ways, so
parseFavorite puts both in filter.Query and favoriteByQuery stops taking
a query it would only overwrite. Giving both spellings at once is
refused: the grammar would OR them, but on a command that mutates
favorites a surprise is worse than a refusal.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

### Task 8: Documentation

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing.

- [ ] **Step 1: Rewrite the filtering section of `README.md`**

Replace the filter examples (around lines 30-45) with:

````markdown
### Filtering

Filters follow one rule: **values within a field OR together, different fields
AND together, and any `--exclude-` match removes the record outright.**

```bash
# Filter by year, decade, or range
disc-fortune --year 1975
disc-fortune --year 1970-1980
disc-fortune --decade 70s

# Filter by genre, label, or format
disc-fortune --genre jazz
disc-fortune --label blue-note
disc-fortune --format 12\"

# --format also matches a pressing's colour
disc-fortune --format "blue translucent"

# Search the whole "Artist - Title" line, or one field of it
disc-fortune --query "kind of blue"
disc-fortune --artist "miles davis"
disc-fortune --title "kind of blue"

# Repeat a flag for "either" -- these are the same records, in one command
disc-fortune --genre jazz --genre funk

# Every filter has an --exclude- twin
disc-fortune --exclude-genre rock
disc-fortune list --decade 70s --exclude-label "blue note"

# Different fields narrow each other
disc-fortune --year 1970-1980 --genre jazz
```

`--year` and `--decade` are two spellings of one field, so they widen rather
than narrow each other: `--year 1959 --decade 70s` gives you 1959 *or* the
seventies.

An exclusion only removes records that actually match it. Discogs leaves the
year or label blank on plenty of releases, and those records survive
`--exclude-year 1975` and `--exclude-label x` rather than quietly disappearing.

Two-digit decades from `30s` to `90s` mean the twentieth century. `--decade
20s` is refused, because it could mean either the 1920s or the 2020s — write
whichever you meant in full.
````

- [ ] **Step 2: Update the feature list**

Change the "Flexible filtering" bullet (around line 208):

```markdown
- **Flexible filtering** - Filter by query, artist, title, year, decade, genre, label, or format; repeat any flag to mean "either", and exclude with `--exclude-genre` and friends
```

- [ ] **Step 3: Verify the documented commands actually work**

```bash
go build -o /tmp/df-t6 .
/tmp/df-t6 help pick
/tmp/df-t6 list --genre jazz --genre funk --exclude-label "blue note" | head
/tmp/df-t6 list --decade 20s
```
Expected: help lists every filter; the OR/exclude command runs; the last one fails with the ambiguity message naming 1920s and 2020s.

- [ ] **Step 4: Run the whole suite one last time**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: document the filter grammar

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01AXsQz6D6Dg9RLvFJmdJDST"
```

---

## What this plan does not do

- **No release.** v2.4.0 "Composability" ships after T7 (`--json`) and T8 (shell completion) land. No version bump, no release notes here.
- **No changes to `--favorites`, `--unheard` or `--draw`.** They are pool filters and draw strategies, not field filters.
- **No completion of filter values.** That is T8, and it should enumerate `filterFields` rather than hardcode a list.
