#!/usr/bin/env bash
# Copyright IBM Corp. 2019, 2026
# SPDX-License-Identifier: MPL-2.0

# corpus.sh compares swissshepherd's findings at a base revision with the
# working tree, on a real provider (AGENTS.md, "Corpus validation").
#
#   PROVIDER_DIR=/path/to/terraform-provider-aws scripts/corpus.sh [BASE]
#
# BASE defaults to main. Uncommitted changes are part of the comparison.
# Each config is run once with the base build and twice with the working-tree
# build, to check determinism. The provider's cached schema is reused; the
# script never passes --refresh-schema.
#
# Environment:
#   PROVIDER_DIR  provider checkout to run in (required)
#   CONFIGS       config files, relative to PROVIDER_DIR
#                 (default: .ci/swissshepherd-full.hcl .ci/swissshepherd-weak.hcl)
#   CORPUS_OUT    output directory (default: $TMPDIR/swissshepherd-corpus)
#
# For each config NAME (swissshepherd-full.hcl -> full), CORPUS_OUT gets
# base-NAME.txt, new-NAME.txt, added-NAME.txt, and removed-NAME.txt, all
# sorted with LC_ALL=C. Locale collation once made comm report findings that
# didn't exist.

set -euo pipefail
export LC_ALL=C

base=${1:-main}
: "${PROVIDER_DIR:?set PROVIDER_DIR to the provider checkout}"
configs=${CONFIGS:-.ci/swissshepherd-full.hcl .ci/swissshepherd-weak.hcl}
tmp=${TMPDIR:-/tmp}
out=${CORPUS_OUT:-${tmp%/}/swissshepherd-corpus}

repo=$(git rev-parse --show-toplevel)
base_sha=$(git -C "$repo" rev-parse --short "$base^{commit}")
head_sha=$(git -C "$repo" rev-parse --short HEAD)
if [[ -n $(git -C "$repo" status --porcelain) ]]; then
  head_sha+="+uncommitted"
fi

work=$(mktemp -d)
cleanup() {
  git -C "$repo" worktree remove --force "$work/base" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

git -C "$repo" worktree add --quiet --detach "$work/base" "$base_sha"
(cd "$work/base" && go build -o "$work/ss-base" .)
(cd "$repo" && go build -o "$work/ss-new" .)

mkdir -p "$out"
cd "$PROVIDER_DIR"
echo "base $base_sha, new $head_sha, provider $(git rev-parse --short HEAD 2>/dev/null || echo '?'), output $out"

# run BIN CONFIG FILE writes sorted output to FILE. swissshepherd exits 1 for
# findings and for failures alike, so a run counts only if it printed its
# summary line.
run() {
  "$1" --config "$2" >"$3.raw" 2>&1 || true
  if ! grep -qE '^([0-9]+ error\(s\)|All checks passed)' "$3.raw"; then
    echo "error: $1 --config $2 failed:" >&2
    head -20 "$3.raw" >&2
    exit 1
  fi
  sort "$3.raw" >"$3"
  rm "$3.raw"
}

count() { grep -c "^$1" "$2" || true; }

for cfg in $configs; do
  name=$(basename "$cfg" .hcl)
  name=${name#swissshepherd-}
  run "$work/ss-base" "$cfg" "$out/base-$name.txt"
  run "$work/ss-new" "$cfg" "$out/new-$name.txt"
  run "$work/ss-new" "$cfg" "$work/again.txt"
  det=deterministic
  cmp -s "$out/new-$name.txt" "$work/again.txt" || det=NONDETERMINISTIC
  comm -13 "$out/base-$name.txt" "$out/new-$name.txt" >"$out/added-$name.txt"
  comm -23 "$out/base-$name.txt" "$out/new-$name.txt" >"$out/removed-$name.txt"
  printf '%s: %s | ERROR %s -> %s, WARN %s -> %s | added ERROR %s WARN %s | removed ERROR %s WARN %s\n' \
    "$name" "$det" \
    "$(count ERROR "$out/base-$name.txt")" "$(count ERROR "$out/new-$name.txt")" \
    "$(count WARN "$out/base-$name.txt")" "$(count WARN "$out/new-$name.txt")" \
    "$(count ERROR "$out/added-$name.txt")" "$(count WARN "$out/added-$name.txt")" \
    "$(count ERROR "$out/removed-$name.txt")" "$(count WARN "$out/removed-$name.txt")"
  [[ $det == deterministic ]] || exit 1
done
