# T6 — Richer filters

**Date:** 2026-09-03
**Phase:** 4 — v2.4.0 "Composability"
**Status:** Approved design. Implementation plan to follow.
**Roadmap:** [`2026-08-26-roadmap.md`](2026-08-26-roadmap.md), task T6.

---

## Problem

`Filter` (`filter.go:10`) holds five scalar strings and one int. Every narrowing
field takes exactly one value, so a collector who wants "jazz or funk" has to run
the command twice and concatenate, and one who wants "anything but the rock" has
no way to say it at all. `--year` is the only field with any internal structure,
and only because it grew a range parser.

Three gaps follow from that single shape:

1. **No OR.** `--genre jazz` and `--genre funk` cannot coexist; the second
   silently wins, because Go's flag package overwrites a repeated string flag.
2. **No negation.** A 900-record collection with 300 rock records has no way to
   ask for the other 600.
3. **No field-specific text search.** `--query` matches `Album.Key()`, which is
   `"Artist - Title"` — so a search for the artist *Genesis* also returns every
   album *titled* Genesis, and there is no way to separate them. Worse, `--query`
   is not a flag at all: it is the positional `QUERY` on `favorite` and
   `unfavorite` only. `pick` and `list` reject positional arguments outright
   (`cli.go:181`), so they have **no free-text search of any kind** today.

The roadmap is explicit that this is permanent CLI grammar and must be settled in
writing before implementation, because it cannot be changed once scripts depend on
it. That is what this document settles.

## Constraints inherited from earlier phases

These are not preferences. Each is a rule an earlier phase paid for.

- **Shared registration, central help.** Phase 1's T3 established that a flag
  every command should accept is registered in one place, and that its help text
  is emitted centrally so no command can ship undocumented. `addFilterFlags`
  (`cli.go:69`) and `filterFlagHelp` (`cli.go:446`) are that mechanism for
  filters, and `TestFilterFlagsAreDocumented` is what enforces it. This design
  extends both rather than working around either.
- **`anyNarrowing()` vs `identifies()`.** v2.2.1 split the filter flags into
  those that only *refine* a query (`--year`, `--genre`, `--label`, `--format`)
  and the one that *identifies* a record outright (`--release-id`). The roadmap
  names this "the shape any further identifying flag should follow". Every flag
  added here is narrowing; `--release-id` remains the only identifying one, and
  it is the only filter flag this task does not touch.
- **`Album.Key()` is not an identity.** Phase 2 recorded that changing `Key()`
  would silently break every query. `--query` continues to match `Key()` exactly
  as the positional query does today, and `--artist`/`--title` are additions
  beside it, never a redefinition of it.

## Design

### 1. The rule

The entire grammar is one sentence:

> **Values within a field OR together; different fields AND together; any
> `--exclude-*` match removes the album outright.**

Corollaries, all deliberate:

- `--genre jazz --exclude-genre jazz` yields nothing. Exclusion wins over
  inclusion, always. This is not a conflict to be resolved; it is a filter that
  happens to be empty, the same as `--year 1975 --year 1976` on a collection with
  neither.
- **Exclusion only drops albums that match.** An album with no year survives
  `--exclude-year 1975`, and one with no label survives `--exclude-label x`.
  Absence is not a match. This matters because Discogs leaves `label` and `year`
  empty on plenty of real entries -- both are `omitempty` on `Album`
  (`collection.go:24-25`) precisely because they are often missing -- and the
  opposite rule would let a single exclusion quietly delete every such record
  from the results.
- For the list-valued album fields, matching is any-element: `--genre jazz`
  passes an album tagged `[Jazz, Funk]`, and `--exclude-genre rock` drops an
  album tagged `[Rock, Jazz]` even though it is also jazz.
- **`--decade` feeds the same constraint as `--year`.** They are two spellings of
  one field, so `--year 1959 --decade 70s` means "1959 or the 70s" rather than the
  empty intersection that treating them as separate AND-ed fields would produce.
  `--exclude-decade` likewise appends to the year exclusions.

### 2. Syntax, and why

Two spellings were available for each of the two new capabilities. Both choices
are permanent, so both are recorded with their rejected alternatives.

**Repetition, not comma-separation.** `--genre jazz --genre funk`. The rejected
alternative, `--genre jazz,funk`, cannot express a value containing a comma —
and label text routinely does (`Blue Note, Inc.`), as does the free text of
`formats[]` that `--format` searches. Accepting both spellings was also rejected:
it is two grammars for one idea, it inherits the comma hazard anyway, and it
forces T8's completion to reason about partial comma lists.

**`--exclude-NAME`, not `--no-NAME` or a `!` value prefix.** `--exclude-genre
rock` reads correctly when repeated and cannot be misread the way `--no-genre`
can — the latter looks like a request for albums that have no genre at all. The
`!` prefix (`--genre '!rock'`) keeps the flag surface half the size, but `!`
triggers history expansion in interactive bash and zsh, so every negated filter
would need quoting and a forgotten quote fails with `zsh: event not found`
rather than anything about disc-fortune. A CLI meant to be typed should not have
a foot-gun in its most common new operation.

### 3. Data model — `filter.go`

```go
// FieldFilter is one field's constraint: values to require (any of), and
// values that disqualify. Both empty means unconstrained.
type FieldFilter struct {
	Include []string
	Exclude []string
}

// YearFilter is the same shape over parsed ranges rather than substrings,
// because --year compares numerically. --decade appends to it.
type YearFilter struct {
	Include []yearRange
	Exclude []yearRange
}

// yearRange is an inclusive span. A single year is a range of one.
type yearRange struct{ start, end int }

type Filter struct {
	Query, Artist, Title, Genre, Label, Format FieldFilter
	Year                                       YearFilter
	// ReleaseID is unchanged: it identifies one exact record, so it stays
	// a single int compared whole. See anyNarrowing/identifies.
	ReleaseID int
}
```

`FieldFilter.matches(values []string) bool` is the whole matching engine for the
six substring fields:

1. If any `Exclude` value is a case-insensitive substring of any of `values`,
   return false.
2. If `Include` is non-empty and no `Include` value is a substring of any of
   `values`, return false.
3. Otherwise true.

Scalar album fields pass a one-element slice; `Genres` and `Formats` pass their
own. `YearFilter.matches(year int) bool` is the same three steps over range
containment, with a zero year matching nothing and therefore being excluded by
nothing.

### 4. The field table

A package-level table is the single source of truth for the six substring
fields — flag name, help line, how to read the album's values, and which
`FieldFilter` it fills:

```go
type filterField struct {
	name       string                    // "genre", giving --genre and --exclude-genre
	help       string                    // one line in the shared help block
	albumValue func(Album) []string      // what to match against
	part       func(*Filter) *FieldFilter // where parsed values land
}
```

Flag registration, help generation, and `Filter.matches` all loop over this
table, so a seventh substring filter is one entry rather than four edits in four
files. Year is deliberately *not* in the table: it parses its values, compares
numerically, and has two flag names feeding it, so forcing it into the substring
shape would cost more than the duplication saves. It is handled explicitly
beside the loop.

### 5. CLI surface — `cli.go`

`addFilterFlags` grows from 5 flags to 17: eight narrowing, eight `--exclude-*`
twins, and the untouched `--release-id`. Every one is built from the table (or,
for year, beside it) using the existing `arrayFlags` type that `--folder`
already uses (`sync.go:25`), which is precisely the repeatable-string-flag
adapter this needs.

Help stays legible by naming the twin convention once instead of listing sixteen
lines:

```
Filters (all repeatable; each has an --exclude-NAME twin that removes matches):
  --query VALUE    Filter by "Artist - Title" (case-insensitive substring)
  --artist VALUE   Filter by artist
  --title VALUE    Filter by title
  --genre VALUE    Filter by genre
  --label VALUE    Filter by label
  --format VALUE   Filter by format or colour
  --year VALUE     Filter by year or year range (e.g., 1975 or 1970-1980) (repeatable)
  --decade VALUE   Filter by decade (e.g., 70s or 1970s); adds to --year (repeatable)
  --release-id N   Select one exact record by its Discogs release ID (single-valued, no twin)
```

`--release-id` is listed under the same heading for completeness, since it is
still a filter flag every filter-taking command accepts, but the heading's
"all repeatable ... each has a twin" claim does not hold for it: it identifies
one exact record rather than narrowing a query, so it takes one value and has
no `--exclude-release-id`. Its own line says so, rather than the heading
carving out an exception that is easy to stop noticing.

`TestFilterFlagsAreDocumented` gets **stricter**, not weaker. It walks the
registered flag set and requires each flag to be either documented literally in
the block or to be the `--exclude-` twin of a flag that is, and separately
requires the twin sentence to be present. A new filter still cannot ship
undocumented, and a twin cannot ship without the sentence that explains it.

### 6. `--decade` parsing

Accepted: `1970s`, `1970`, and the two-digit forms `30s` through `90s`, which
are unambiguous because there are no 2030s pressings yet. Each yields the
inclusive range `[N0, N9]`.

Rejected, with an error naming both spellings: `00s`, `10s`, `20s`.

```
$ disc-fortune list --decade 20s
list: ambiguous decade "20s": write 1920s or 2020s
```

The rejected alternatives were "two digits always mean 19xx", which makes
`--decade 20s` permanently unable to reach a 2020s pressing, and "the most
recent decade that has already begun", which is friendlier today but silently
changes the meaning of `--decade 30s` in 2030 and forces every test to run
against a fixed clock. A fixed rule that refuses the three genuinely ambiguous
inputs is the only one that is both stable and honest.

### 7. `--query`, and the positional query

`--query` becomes a real filter flag, available on every command that takes
filters. On `pick` and `list` this is new capability: `disc-fortune list --query
miles` is a free-text search those commands have never had.

On `favorite` and `unfavorite` the positional `QUERY` remains, and is exactly
equivalent to a single `--query` value. Two consequences:

- **A `--query` satisfies "filters require a query."** `favorite --query miles`
  works; `favorite --genre jazz` still fails with the existing message, because
  a genre does not say which record is meant. Concretely, `anyNarrowing()` stops
  counting the query field and a new `hasQuery()` reports it, so the existing
  check at `cli.go:236` becomes "narrowing without any query".
- **Giving both spellings at once is an error.** `favorite "miles" --query
  coltrane` fails with `favorite: give the query once, as an argument or
  --query`. OR-ing them would be defensible by the rule in §1 but would surprise
  anyone who typed both by accident, and on a command that mutates favorites a
  surprise is worse than a refusal.

`--exclude-query` exists for uniformity and is genuinely useful on `list`
(`--query miles --exclude-query bootleg`). It never satisfies `hasQuery()`: an
exclusion says which record is *not* meant.

### 8. Empty values stay a no-op

`--genre ""` continues to mean "no genre filter", as it does today. Empty values
are dropped at parse time, for both includes and excludes.

The alternative — rejecting an empty value with an error — is more informative,
but `--genre "$GENRE"` with an unset variable is a common shell idiom that works
today, and turning it into a hard failure would break working scripts for no
gain. Dropping empties is also what makes `--exclude-genre ""` harmless:
`strings.Contains(anything, "")` is true, so an empty exclusion that reached the
matcher would exclude the entire collection.

### 9. Explicitly out of scope

- **`--release-id` stays single-valued.** Repeating it would mean "these two
  exact records", which contradicts the v2.2.1 contract that it identifies one
  record and needs no query, and would complicate the ambiguity path in
  `runFavorite` for no demonstrated need.
- **No cross-field boolean expressions.** There is no way to say "(jazz AND
  1970s) OR (funk AND 1980s)". The one-sentence rule in §1 is the ceiling. A
  general expression grammar is a different feature with a different design, and
  YAGNI applies.
- **No parentheses, no `--or`/`--and` connectives, no regex.** Substring matching
  stays exactly as it is.
- **`--favorites` and `--unheard` are untouched.** They are pool filters, not
  field filters, and Phase 3 fixed their semantics deliberately.

## Acceptance

- `--genre jazz --genre funk` returns albums matching either; the pre-T6
  behavior of silently keeping only the last value is gone.
- `--exclude-genre rock` returns every album not tagged rock, including albums
  with no genres at all.
- `--exclude-label x` does not drop albums whose label is empty; `--exclude-year
  1975` does not drop albums whose year is zero.
- `--year 1959 --decade 70s` returns 1959 *and* the 1970s, not nothing.
- `--decade 20s` fails with a message naming both 1920s and 2020s; `--decade
  70s`, `1970s` and `1970` are all accepted and identical.
- `--artist genesis` and `--title genesis` return different sets on a collection
  containing both.
- `list --query miles` works, having previously been impossible.
- `favorite --query miles` favorites; `favorite --genre jazz` still errors with
  "filters require a query"; `favorite "miles" --query coltrane` errors.
- `--genre ""` behaves as it does today: no genre filter.
- Every single-value invocation documented in the v2.3.0 README returns byte-
  identical output. This is the backward-compatibility guarantee.
- Every registered filter flag is documented or is a documented flag's twin.

## Tests

TDD throughout; each bullet is a failing test before it is code.

- **`FieldFilter.matches` table:** include-only, exclude-only, both, neither,
  multi-valued album fields, absent/empty album fields, case-insensitivity.
- **`YearFilter.matches` table:** single year, range, reversed range (the
  existing auto-swap), decade-derived range, zero year against both an include
  and an exclude.
- **Decade parse table:** `70s`, `1970s`, `1970`, `2020s` accepted; `00s`, `10s`,
  `20s` rejected with a message naming both spellings; garbage rejected with the
  existing style of message.
- **Grammar integration:** a small fixture collection asserting OR-within-field
  and AND-across-field in one command, and that exclusion beats inclusion.
- **Empty-value no-op:** `--genre ""` matches everything; `--exclude-genre ""`
  excludes nothing.
- **Registration:** every table entry produces both a narrowing and an
  `--exclude-` flag on every filter-taking command (`pick`, `list`, `favorite`,
  `unfavorite`).
- **Documentation:** the strengthened `TestFilterFlagsAreDocumented`.
- **Query plumbing:** positional and `--query` equivalence, the both-given error,
  `--exclude-query` not satisfying `hasQuery()`, and `favorite --genre jazz`
  still failing.
- **Regression:** the existing `filter_test.go` cases, updated only where the
  struct literal shape forces it, never where the semantics should hold.

## Files

- `filter.go` — `FieldFilter`, `YearFilter`, `yearRange`, the field table, the
  rewritten `Filter.matches`, decade parsing.
- `cli.go` — `addFilterFlags`, `filterFlags.Filter()`, `anyNarrowing()`,
  `hasQuery()`, `filterFlagHelp`, the `favorite`/`unfavorite` query check.
- `filter_test.go`, `cli_test.go` — per the section above.
- `README.md` — the filtering section, plus the new `--artist`/`--title`/
  `--decade`/exclusion examples.

## Downstream

T7 (`--json`) and T8 (shell completion) both depend on this landing first. T8 in
particular reads the flag surface, and the field table in §4 is what it should
enumerate rather than hardcoding a list that can drift.
