#!/usr/bin/env zsh
# Compares the current tree's binary against a baseline build across a fixed
# set of invocations, asserting stdout, stderr and exit code are identical.
#
# Usage: scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base
set -uo pipefail

BASE="${1:?usage: behaviour-diff.sh <baseline-binary>}"
BASE="${BASE:A}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

go build -o "$WORK/new" . || { echo "BUILD FAILED"; exit 1; }

# A populated config dir, byte-identical for both binaries.
FIXTURE="$WORK/home"
mkdir -p "$FIXTURE/.config/disc-fortune"
cat > "$FIXTURE/.config/disc-fortune/collection.json" <<'JSON'
[
  {"release_id":1,"artist":"Miles Davis","title":"Kind of Blue","year":1959,"label":"Columbia","catno":"CL 1355","genres":["Jazz"],"formats":["Vinyl","LP"]},
  {"release_id":2,"artist":"Alice Coltrane","title":"Journey in Satchidananda","year":1971,"label":"Impulse!","catno":"AS-9203","genres":["Jazz"],"formats":["Vinyl","LP"]},
  {"release_id":3,"artist":"Parliament","title":"Mothership Connection","year":1975,"label":"Casablanca","catno":"NBLP 7022","genres":["Funk"],"formats":["Vinyl","LP"]}
]
JSON
cat > "$FIXTURE/.config/disc-fortune/favorites.json" <<'JSON'
[{"release_id":2,"artist":"Alice Coltrane","title":"Journey in Satchidananda","year":1971,"label":"Impulse!","catno":"AS-9203","genres":["Jazz"],"formats":["Vinyl","LP"]}]
JSON
cat > "$FIXTURE/.config/disc-fortune/history.json" <<'JSON'
[{"album":{"release_id":1,"artist":"Miles Davis","title":"Kind of Blue","year":1959,"label":"Columbia","catno":"CL 1355","genres":["Jazz"],"formats":["Vinyl","LP"]},"timestamp":"2026-09-01T10:00:00Z"}]
JSON
cat > "$FIXTURE/.config/disc-fortune/meta.json" <<'JSON'
{"synced_at":"2026-09-01T10:00:00Z"}
JSON

# Probes: each line is one argv. Deterministic commands only -- `pick` is
# excluded from the identical-output set because it draws at random; it is
# covered by exit-code-only probes below.
PROBES=(
  "list"
  "list --json"
  "list --genre jazz"
  "list --genre nope"
  "list --favorites"
  "history"
  "history --json"
  "history 1"
  "stats"
  "stats --json"
  "stats --favorites"
  "stats --genre nope"
  "help"
  "help pick"
  "help stats"
  "version"
  "--color=always list"
  "--color=never list"
  "open --print --release-id 1"
  "open --print --release-id 999"
  "completion bash"
  "completion zsh"
  "completion fish"
  "bogus-command"
  "list --bogus-flag"
  "favorite"
  "unfavorite --release-id 999"
)

FAIL=0
run_one() {  # $1=binary $2=argv-string $3=outfile
  local home_copy="$WORK/run-home"
  rm -rf "$home_copy"; cp -R "$FIXTURE" "$home_copy"
  ( cd "$home_copy" && env -i HOME="$home_copy" PATH="$PATH" \
      "$1" ${=2} ) >"$3.out" 2>"$3.err"
  echo "$?" > "$3.code"
}

for p in "${PROBES[@]}"; do
  run_one "$BASE"      "$p" "$WORK/base"
  run_one "$WORK/new"  "$p" "$WORK/new_"
  for ext in out err code; do
    if ! diff -q "$WORK/base.$ext" "$WORK/new_.$ext" >/dev/null; then
      echo "MISMATCH [$ext] for: disc-fortune $p"
      diff -u "$WORK/base.$ext" "$WORK/new_.$ext" | sed 's/^/    /'
      FAIL=1
    fi
  done
done

# `pick` is random: assert only that the exit code and stderr shape match.
for p in "pick" "pick --favorites" "pick --genre nope" "pick --json"; do
  run_one "$BASE"     "$p" "$WORK/base"
  run_one "$WORK/new" "$p" "$WORK/new_"
  if ! diff -q "$WORK/base.code" "$WORK/new_.code" >/dev/null; then
    echo "MISMATCH [exit code] for: disc-fortune $p"
    FAIL=1
  fi
done

if [ "$FAIL" -eq 0 ]; then echo "behaviour-diff: OK (${#PROBES[@]} probes + 4 pick probes)"; fi
exit "$FAIL"
