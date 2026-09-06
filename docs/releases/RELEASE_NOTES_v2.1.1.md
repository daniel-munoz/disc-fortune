# disc-fortune v2.1.1

**No functional changes.** This release fixes the module path so that v2 can
actually be installed with `go install`. If you build from a clone, nothing
about this release affects you.

## v2.0.0 and v2.1.0 were not installable

Go requires that a module at v2 or above carry its major version in the module
path. `go.mod` was never updated when the project moved to v2, so it continued
to declare:

```
module github.com/daniel-munoz/disc-fortune
```

while the repository was tagged `v2.0.0` and `v2.1.0`. That combination is not
valid under either path, and both tags were rejected by the module system:

```sh
$ go install github.com/daniel-munoz/disc-fortune@v2.1.0
invalid version: module contains a go.mod file, so module path must match
major version ("github.com/daniel-munoz/disc-fortune/v2")

$ go install github.com/daniel-munoz/disc-fortune/v2@v2.1.0
404 not found: go.mod has non-.../v2 module path
"github.com/daniel-munoz/disc-fortune" (and .../v2/go.mod does not exist)
```

Because neither v2 tag was a valid version of the unsuffixed path,
`go install github.com/daniel-munoz/disc-fortune@latest` resolved to v1.3.0 —
correctly, but confusingly, since v2 had been released twice by then.

`go.mod` now declares `github.com/daniel-munoz/disc-fortune/v2`, and v2.1.1 is
the first v2 release that installs:

```sh
go install github.com/daniel-munoz/disc-fortune/v2@latest
```

The `v2.0.0` and `v2.1.0` tags and their release notes are left in place as
history. They remain uninstallable and cannot be repaired in place — a published
tag's `go.mod` is what the module system reads, so the fix necessarily arrives
as a new version.

## The `/v2` suffix is now part of the import path, permanently

`go install github.com/daniel-munoz/disc-fortune@latest`, without the suffix,
will continue to install the newest **v1** release for as long as v1 tags exist.
This is how Go distinguishes major versions and is not something a future
release can change. v2 users need the `/v2`.

The v1 line stays maintained on its own branch and keeps the unsuffixed module
path, so v1 users continue to receive fixes at the address they already use.
