# disc-fortune v2.6.0 — "Second Thoughts"

One new command. The first release after the post-v2.0.0 roadmap closed.

## `reroll`

Draws a replacement for your last pick and writes it over that pick's history
entry, instead of adding a new one:

```bash
# Not what you wanted to hear
disc-fortune reroll

# Reroll, and narrow while you are at it
disc-fortune reroll --genre jazz
disc-fortune reroll --favorites --draw stale
```

**Why this needed a command rather than just picking again.** Since v2.3.0,
`history.json` is not a log — it is the input to every decision the tool makes.
`--draw fresh` avoids what you played recently, `--draw stale` favors what you
have left longest, `--unheard` is defined entirely by it, and `stats` reports
the share of a set you have ever played from it.

So a record you were handed, looked at, and declined to play was recorded as
though you had listened to it end to end. It was then avoided for your next
several picks and dropped off `--unheard` permanently — the opposite of what
you want, since you still have not heard it. Picking again only compounded it:
two records marked played, one of them actually played.

`reroll` removes the declined entry before it draws, so the record goes back
into the pool as though it had never come up.

## Notes

- `reroll` takes every flag `pick` takes, including `--favorites`,
  `--unheard`, `--draw`, `--json` and all the filters.
- The record you turned down is eligible for the replacement draw, so with a
  narrow filter a reroll can hand it straight back. Reroll again — it costs
  nothing.
- It prints `Replaced: Artist - Title (2 minutes ago)` on stderr. That is a
  receipt for a destructive action rather than advice, so unlike the
  sync-staleness notice it appears even when stderr is not a terminal.
- `reroll --json` emits exactly the payload `pick --json` emits. The
  replacement is reported on stderr only.
- Rerolling something you already favorited does not unfavorite it;
  `favorites.json` is untouched. `favorite`, `unfavorite` and `open` with no
  query mean "the last pick", which after a reroll is the new album.
- There is no undo. A reroll days after the fact will erase a listen that
  really happened — the `Replaced:` line names what it took.

## Under the hood

`pick` and `reroll` share one `drawAndRecord` path, differing only in whether
the last history entry is dropped before the draw and which writer records the
result. The write itself is a compare-and-swap: `reroll` decides from a history
it reads without the lock, so `ReplaceLastHistory` refuses if the last entry is
no longer the one it was shown, rather than silently deleting a different
record. Nothing about `history.json`'s format changed, so downgrading to v2.5.0
loses nothing.
