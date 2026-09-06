# disc-fortune v2.4.0 — "Composability"

**Nothing you already do changes.** Unlike v2.3.0, this release alters no
default and no exit code. Every command behaves exactly as it did in v2.3.0
unless you reach for something new. The pre-release binary and this one were
built side by side and compared across every non-`--json` invocation we could
construct — byte-identical stdout, stderr and exit codes, ANSI escapes
included.

This release is about composing: richer filters to say what you want, JSON to
hand the answer to something else, and completion so you can type it.

## Filters compose now

One rule governs all of it:

> **Values within a field OR together; different fields AND together; any
> `--exclude-` match removes the record outright.**

```sh
# repeat a flag to mean "either"
disc-fortune --genre jazz --genre funk

# every narrowing filter has an --exclude- twin
disc-fortune --exclude-genre rock
disc-fortune list --decade 70s --exclude-label "blue note"

# new fields: search the artist or the title, not both at once
disc-fortune --artist genesis     # the band
disc-fortune --title genesis      # the album

# --query is now a flag, so pick and list can search at all
disc-fortune list --query "kind of blue"
```

`--year` and `--decade` are two spellings of one field, so they widen rather
than narrow each other: `--year 1959 --decade 70s` gives you 1959 *or* the
seventies, not the empty intersection.

**An exclusion only removes records that actually match it.** Discogs leaves
the year or label blank on plenty of releases, and those survive
`--exclude-year 1975` and `--exclude-label x` rather than quietly disappearing.

Two-digit decades from `30s` to `90s` mean the twentieth century. `--decade
20s` is refused rather than guessed at:

```
$ disc-fortune list --decade 20s
list: ambiguous decade "20s": write 1920s or 2020s
```

`--release-id` is unchanged and deliberately excluded from all of this: it
identifies one record rather than narrowing a query, so it stays single-valued
with no `--exclude-` twin.

## `--json`

`pick`, `list` and `history` accept `--json`:

```sh
disc-fortune pick --json
disc-fortune list --json --genre jazz | jq -r '.albums[] | "\(.artist) - \(.title)"'
disc-fortune history --json 5
```

Each emits one JSON object. Every album carries all eight keys, always —
`release_id`, `artist`, `title`, `year`, `label`, `catno`, `genres`,
`formats` — with `null` for what Discogs did not tell us and `[]` for absent
lists, so you can model one fixed type rather than branching on which keys
turned up.

**`--json` changes the format and nothing else.** Exit codes, the messages on
stderr, and `pick` recording its pick in `history.json` are identical either
way. That is worth knowing before you script it, because it means an empty
result is still a failure:

```sh
# exits 1 with an empty stdout, not {"albums": [], "count": 0}
disc-fortune list --json --genre nonexistent
```

so check the exit code:

```sh
if out=$(disc-fortune list --json --genre jazz); then
  echo "$out" | jq '.count'
fi
```

`history --json` on an empty history exits 0 with an empty payload. The two
disagree, and they disagreed before this release — the JSON mirrors the
existing exit codes rather than quietly reconciling them.

Timestamps are RFC 3339 with your **local UTC offset, not `Z`**, and their
fractional seconds are variable-length. Parse them with a real RFC 3339
parser rather than a fixed format string.

## Shell completion

```sh
eval "$(disc-fortune completion bash)"
eval "$(disc-fortune completion zsh)"
disc-fortune completion fish | source
```

The script is generated from the commands and flags the binary actually
accepts, so it cannot drift from them. Flags are scoped to the command that
takes them: `list --<TAB>` offers `--json` but not `--draw`, because `list`
rejects `--draw`.

Command names, flag names, and the fixed values of `--draw` and `--color` are
completed. Values that would have to be read from your collection — those of
`--genre` and `--label` — are not: a tab-press should never depend on a file
that a `sync` may be rewriting.

## Known limitations

- **Completion offers no values from your collection.** Deliberate, per above.
  If you want `--genre <TAB>` to list your genres, that needs its own design:
  what happens when the collection is missing, being rewritten, or large
  enough that a tab-press feels slow.
- **`--json` is on three commands only.** `favorite`, `unfavorite`, `sync` and
  `folders` reject it rather than accepting and ignoring it. They can be added
  later under the same schema.
- **The filter grammar has a ceiling.** Values OR within a field, fields AND
  together, exclusions remove — and that is all. There are no parentheses, no
  cross-field boolean expressions, and no regex. Anything richer needs its own
  design rather than an extension of this one.
