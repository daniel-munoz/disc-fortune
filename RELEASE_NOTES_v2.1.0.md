# disc-fortune v2.1.0 — "Durability"

**No breaking changes.** Every v2.0.0 invocation behaves exactly as before. This
release closes the defects found in the post-v2.0.0 review: nothing here is a new
capability, and everything here is something that could previously have cost you
data or left you staring at a hung terminal.

## Your data can no longer be corrupted by an interrupted write

`collection.json`, `history.json` and `favorites.json` were each written with a
direct `os.WriteFile` over the live path. That call truncates the file before it
writes, so a crash, a full disk, or a `^C` at the wrong moment left a half-written
file behind.

`history.json` made this more than theoretical: it is rewritten on *every pick*,
making it the highest-frequency write in the tool. A truncated one fails to parse,
and every command that reads it — including the default `disc-fortune` with no
arguments — stayed broken until you found and deleted the file by hand.

All data files are now written atomically: to a temporary file in the same
directory, flushed to disk, then renamed into place. A failed write leaves the
previous file byte-for-byte intact.

File permissions behave exactly as they did before: a file being created gets
`0644` as filtered by your umask, so `umask 077` still yields a private
collection; and a file that already exists keeps whatever mode it has, so if
you have tightened `history.json` yourself it stays tightened.

## Syncing survives rate limits, and tells you it is alive

Four fixes in one code path:

- **Rate limits are retried.** Discogs allows 60 authenticated requests per
  minute and answers a breach with `429`. That used to surface as a raw response
  body inside an error and abort the entire sync. Rate limits and server errors
  are now retried with exponential backoff — starting at 1s, doubling, with
  jitter — honoring the server's `Retry-After` when it asks for longer. Retries
  are bounded, so a genuinely broken endpoint still fails, loudly, in finite time.
  A `404` or `401` is not retried; repeating it would not help.
- **Long syncs report progress.** `sync` used to print nothing until it had
  fetched every page, which on a real collection is a silent multi-minute wait
  with no evidence the process is alive. It now reports each page as it lands.
  Progress goes to stderr, and only when stderr is a terminal, so `stdout`
  remains a clean data channel for piping.
- **The tool identifies itself honestly.** The `User-Agent` said
  `disc-fortune/1.0` while the tool was at 2.0.0. It is now derived from the
  version constant and cannot drift again.
- **Sync times are recorded.** A new `meta.json` holds `synced_at`. If your
  collection has gone more than 90 days without a sync, a pick mentions it once —
  on stderr, only on a terminal.

## `XDG_CONFIG_HOME`, `NO_COLOR`, and `--color`

`XDG_CONFIG_HOME` is now honored, and a relative value is ignored per the
basedir spec.

**If you already have `XDG_CONFIG_HOME` set, nothing moves.** Your data is in
`~/.config/disc-fortune/`, and switching to the XDG path on upgrade would have
made your entire collection appear to vanish. disc-fortune keeps using the
directory that actually holds your data, tells you once that a better location
is available, and leaves the decision to you:

```sh
disc-fortune migrate
```

`migrate` copies each file to the XDG location and removes the originals. It
copies rather than renames, because `XDG_CONFIG_HOME` may be on another
filesystem; each file is written atomically; and it refuses to run if the
destination already contains files, rather than guessing at a merge.

Color is now controllable:

```sh
disc-fortune list --color=always | less -R   # keep color through a pipe
disc-fortune list --color=never              # suppress it on a terminal
```

`--color` defaults to `auto` and is accepted by every command. Under `auto`, a
non-empty [`NO_COLOR`](https://no-color.org) disables color; an explicit
`--color=always` overrides `NO_COLOR`, since that is a direct instruction from
you.

## Note for anyone reading the source

`configDir()` no longer calls `os.Exit(1)`. Path resolution is a pure function of
the environment that returns an error, resolved once at startup. Commands that
touch no data files (`help`, `version`, `folders`) keep working even when the
config directory cannot be resolved at all.
