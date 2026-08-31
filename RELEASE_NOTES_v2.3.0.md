# disc-fortune v2.3.0 — "Discovery"

**The default changed.** Plain `disc-fortune` no longer picks uniformly at
random. It now avoids the records you played most recently, so the same
album does not come back around twice in a week.

```sh
disc-fortune
```

behaves differently today than it did in v2.2.1, on the same collection and
history, with no flag added. If you script against `pick` and need the old,
history-blind behavior exactly, one flag restores it:

```sh
disc-fortune pick --draw any
```

`any` is a uniform draw with history never consulted — byte-for-byte the
v2.2.1 algorithm, not an approximation of it.

## `--unheard`

Restricts `pick` and `list` to albums that have never appeared in your
history:

```sh
disc-fortune pick --unheard
disc-fortune list --unheard --genre jazz
```

When every album matching your other filters has already been played,
`pick --unheard` exits 1 and says so, rather than silently falling back to a
repeat — that would defeat the point of asking for something new:

```
Every album matching your filters has already been played.
Drop --unheard, or try `disc-fortune pick --draw stale` for whatever you have
left longest.
```

`list --unheard` does the same, minus the `--draw` suggestion, since `list`
never draws anything.

## `--draw any|fresh|stale`

`pick` now takes `--draw`, with three values:

- `fresh` (the default) — exclude the recently played.
- `any` — the old behavior: a uniform draw, history ignored entirely.
- `stale` — exclude the recently played, same as `fresh`, then bias toward
  whatever is left unplayed the longest. A record you have never picked
  outranks every played one; among played ones, the least recently played
  wins.

`stale` is `fresh` plus a bias, not an alternative to it — the anti-repeat
guarantee holds under `stale` exactly as it does under `fresh`.

## The exclusion window scales with what you are actually choosing from

The default draw excludes your last `min(10, pool / 3)` *distinct* picks —
distinct so that spinning one record ten times running does not spend the
whole window on it, and measured against the *filtered* pool, not your whole
collection.

That second part matters: `disc-fortune pick --genre jazz` on four matching
albums uses a window of one, not ten, because `4 / 3` is one. A pool of one
or two albums excludes nothing at all. That arithmetic is deliberate, and it
is why a narrow filter can never be narrowed into an empty set — the
exclusion always leaves something to pick.

## `--favorites` is unchanged

`--favorites` stays exactly what it was: a hard filter, picking uniformly
among your favorites with no history-awareness layered on top. Converting it
to a soft bias was considered and rejected — anyone scripting against
today's behavior would break with no warning, and if soft-weighted favorites
turn out to be wanted later, that gets its own flag rather than silently
changing this one.

## `sync` and concurrent picks no longer race

`sync`'s backfill rewrites `history.json` and `favorites.json` wholesale
while a concurrent `pick` or `favorite` appends to the same file. Before this
release, one of those two writes could simply be lost — cosmetic while
history was only a log, but not any more, now that a missing entry changes
which records the next pick avoids. Both operations now take an advisory
lock before touching either file, so they serialize instead of colliding.

You will see new `*.lock` files alongside `history.json` and `favorites.json`
in your config directory. They are scaffolding: they hold no data of yours,
you can leave them alone, and there is nothing to clean up. `migrate` knows
about them too — it neither copies them to the new location nor leaves them
behind in the old one.

## Known limitation

A history entry saved before v2.2.0's release-ID work still has no ID, and
it acts as a wildcard for its "Artist - Title" — matching every pressing of
that title, not just the one you actually played. `--unheard` inherits that:
it stays conservative about two differently-pressed copies of the same album
until a `sync` backfills their release IDs, at which point the entry starts
matching only the pressing it belongs to.
