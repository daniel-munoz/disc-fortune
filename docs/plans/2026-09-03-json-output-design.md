# T7 — `--json` output

**Date:** 2026-09-03
**Phase:** 4 — v2.4.0 "Composability"
**Status:** Approved design. Implementation plan to follow.
**Roadmap:** [`2026-08-26-roadmap.md`](2026-08-26-roadmap.md), task T7.
**Follows:** [`2026-09-03-richer-filters-design.md`](2026-09-03-richer-filters-design.md) (T6, merged).

---

## Problem

Every command prints for a human. `formatAlbum`, `formatList` and `formatHistory`
emit indented text with optional ANSI colour, a trailing count sentence, and a
`History (last N picks):` header. There is no machine-readable form of any of it,
so a script that wants the release ID of tonight's pick has to parse display text
whose shape is a presentation decision, not a contract.

The roadmap is specific about how to fix it: *"Define and document the schema
explicitly rather than letting it fall out of the structs, and include
`release_id` from day one."* Both halves matter. Serialising `Album` directly
would make the wire format a hostage of the storage format. Omitting
`release_id` would repeat the mistake T4 existed to correct, on an interface
that is harder to change than a data file.

## Constraints inherited from earlier phases

These are not preferences. Each is a rule an earlier phase paid for.

- **Stdout is the data channel; stderr is advisory.** Phase 1's T2 established
  it for sync progress. It is what makes `--json` safe to pipe.
- **Flags are registered where they are implemented.** Phase 3 registered
  `--draw` only on `pick`, so `list --draw stale` fails as an unknown flag
  rather than being accepted and silently ignored. `--json` follows that, not
  the global-flag pattern `--color` uses.
- **`--unheard` is a filter, `--draw` is a strategy.** Phase 3 recorded that
  T7's schema documents them in different places: `--unheard` narrows the
  candidate pool before a draw happens, `--draw` decides how the draw is made
  from whatever survives. Neither appears in the payload — see §5.
- **Do not silently change behaviour scripts depend on.** The maintainer's
  dissent in Phase 3 killed a proposal to soften `--favorites`. The same rule
  governs exit codes here.
- **Absence is a real state, represented explicitly.** T6 established that a
  missing year or label is not a match rather than a zero. The schema says the
  same thing with `null` rather than by omitting a key.

## Design

### 1. The three payloads

```jsonc
// disc-fortune pick --json
{"album": { …album… }}

// disc-fortune list --json
{"albums": [ …album…, … ], "count": 2}

// disc-fortune history --json 2
{"entries": [{"album": { …album… }, "timestamp": "2026-09-03T21:45:06.123456789Z"}],
 "count": 1}
```

Every command emits a JSON **object**, never a bare array. The reason is
one-way: a key can be added to an object without breaking a consumer, but a
top-level array can never become an object. For a schema that is meant to be
permanent, that asymmetry decides it. `count` costs one key and is what the
human output already prints.

**`count` is the number of elements actually emitted**, not the number that
exist. `history --json 2` on a file of 400 entries reports `2`.

**`entries` are most recent first**, matching `formatHistory`'s existing reverse
iteration (`history.go:113`). A consumer reading `entries[0]` gets the same
record a human reading the first line does.

**Timestamps are RFC 3339 exactly as stored**, nanoseconds included. The wire
value and the `history.json` value are the same string, so no rounding or
reformatting can make them disagree.

### 2. The album object

One fixed key set, on every album, in this order:

```json
{
  "release_id": 1839278,
  "artist": "Miles Davis",
  "title": "Kind of Blue",
  "year": 1959,
  "label": "Columbia",
  "catno": "CL 1355",
  "genres": ["Jazz"],
  "formats": ["Vinyl", "LP", "Album"]
}
```

| Key | Type | When unknown |
|---|---|---|
| `release_id` | number or null | `null` for entries written before v2.2.0 |
| `artist` | string | always present |
| `title` | string | always present |
| `year` | number or null | `null` when Discogs gave none |
| `label` | string or null | `null` |
| `catno` | string or null | `null` |
| `genres` | array of string | `[]` |
| `formats` | array of string | `[]` |

The key set never varies, so a consumer can model it as a fixed type and tell a
missing value from a typo. `null` distinguishes "Discogs did not say" from a
real value, which `0` and `""` cannot: `"year": 0` sorts before 1959 and
`"release_id": 0` looks like an ID. Lists stay lists so a loop needs no nil
check.

`artist` and `title` are never null. They are the one pair every entry has —
`Album.Key()` is built from them, and it is the legacy identity for
pre-v2.2.0 data.

### 3. An explicit output type, not `Album` re-serialised

The payload types live in a new `json.go` and are converted from `Album`, per
the roadmap. `Album`'s own `omitempty` storage tags are **not touched**: the
on-disk format and the wire format are now two contracts with two owners, which
is the entire point. `release_id` is `omitempty` on disk (correct — an unknown
ID should not be written as `0`) and always present on the wire (correct — a
consumer needs the key).

The cost is a conversion function that must be kept in step with `Album`. The
golden tests (§6) pin the exact bytes of `jsonAlbum` itself, so a renamed key
or a reordered field fails them immediately — but a field added to `Album`
with no corresponding decision in `jsonAlbum` never touches those bytes, so
the golden tests do not catch it and the suite stays green. What does catch
it is `TestEveryAlbumFieldHasAWireDecision`, a reflection test that compares
`Album`'s and `jsonAlbum`'s field counts and order and fails when they
diverge, forcing a conscious choice about the wire format for every new
`Album` field.

### 4. `--json` changes the format, never the semantics

- **Registered on `pick`, `list` and `history` only.** `sync --json` fails as an
  unknown flag rather than being accepted and ignored, exactly as
  `list --draw stale` does.
- **Exit codes are byte-identical to today**, and this is the load-bearing rule
  of this design. `--json` is a formatting flag. Anyone scripting today's exit
  codes keeps working.
- **Advisory and error messages stay plain text on stderr.** No JSON error
  envelope. The exit code already carries the signal, and putting failures on
  stdout would break "stdout is the data channel" for the one command mode that
  most depends on it.
- **When a command exits non-zero, stdout stays empty.** No partial payload.
- **`pick --json` still records the pick in `history.json`.** It is a format
  flag, not a dry run.
- **JSON is never colourised**, whatever `--color` says or `NO_COLOR` does not
  say. An ANSI escape inside a JSON string is a parse hazard for no benefit.
- **Output is two-space indented with a trailing newline.** Readable without
  `jq` installed; `jq` normalises anyway.

Two inherited behaviours the schema mirrors rather than invents, because they
already differ and this is not the task that reconciles them:

| Case | Exit | stdout |
|---|---|---|
| `list --json` matching nothing | **1** (`main.go:166-168`) | empty |
| `history --json` on empty history | **0** (`history.go:101`) | `{"entries": [], "count": 0}` |
| `pick --json --unheard`, all heard | **1** | empty |

`list` treats an empty result as a failure and `history` treats it as a valid
answer. That asymmetry predates this task. Changing either would be a silent
behaviour change to a scripted exit code, which the Phase 3 rule forbids.

### 5. Explicitly out of scope

- **No `--json` on `favorite`, `unfavorite`, `sync`, `folders`, `migrate` or
  `help`.** The roadmap names three commands. The others can be added later
  under this same schema if a need appears; adding them now is speculation.
- **No draw metadata on `pick`.** It is tempting to report that a record was
  chosen because it was the stalest, and `--draw` makes the information
  available — but it is permanent surface with no demonstrated consumer, and
  §4's rule is that `--json` reports *what* the command did, not *why*.
- **No `schema_version` key.** The object envelope means one can be added later
  without breaking a consumer. Adding it now is a guess about a problem nobody
  has, and an unused version field is worse than none: it invites consumers to
  branch on a value that never changes.
- **No filter echo.** `list --json` does not report which filters produced the
  result. The caller passed them and already knows.
- **`--json` does not imply quiet.** Sync progress and other stderr advice are
  unaffected, because they were never on stdout.

## Acceptance

- `pick --json`, `list --json` and `history --json` each emit a single JSON
  object followed by a newline, and nothing else, on stdout.
- Every album carries all eight keys, in the documented order, whatever is
  missing from the underlying record.
- An album with no release ID, year, label, catno, genres or formats emits
  `null`, `null`, `null`, `null`, `[]`, `[]` — and still emits `artist` and
  `title` as strings.
- `history --json 2` on a longer history reports `"count": 2` and returns the
  two most recent entries, most recent first.
- `list --json` matching nothing exits 1 with an empty stdout; `history --json`
  on an empty history exits 0 with `{"entries": [], "count": 0}`.
- `--color=always` with `--json` produces output containing no ESC byte.
- `sync --json` fails as an unknown flag.
- `pick --json` appends to `history.json` exactly as `pick` does.
- Every exit code in the v2.3.0 test suite is unchanged.

## Tests

- **Golden tests** pinning the exact bytes of all three payloads: a fully
  populated album, and one with everything absent. These stop `jsonAlbum`
  drifting from itself -- a renamed key, a reordered field -- but not from
  `Album`: a field added to `Album` with no `jsonAlbum` counterpart changes no
  golden bytes.
- **`TestEveryAlbumFieldHasAWireDecision`**, a reflection test comparing
  `Album`'s and `jsonAlbum`'s field counts and order. This is what stops the
  wire format silently falling out of step when `Album` changes.
- A test that `encoding/json` round-trips what we emit, so the output is not
  merely plausible but parseable.
- A test asserting no `0x1b` byte in `--json` output under `--color=always`.
- `history --json N` ordering and `count` against a fixture of more than N.
- Exit-code tests for the three rows of §4's table.
- A registration test: `--json` accepted by `pick`, `list`, `history`, and
  rejected by every other command.
- The documentation guard extended, so `--json` cannot ship undocumented.

## Files

- New `json.go` — the payload types and the conversion from `Album`.
- New `json_test.go` — golden and round-trip tests.
- `cli.go` — `--json` registration on three commands, and the usage blocks.
- `main.go` — `runPick`, `runList`, `runHistory` branch on the flag.
- `README.md` — a `--json` section documenting the schema.

## Downstream

T8 (shell completion) is unaffected: `--json` is a plain boolean and needs no
value completion. It should still appear in the completion output for the three
commands that accept it, which follows automatically if T8 enumerates the
registered flag set rather than a hardcoded list.
