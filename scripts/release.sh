#!/usr/bin/env bash

set -euo pipefail

usage() {
  printf 'Usage: %s vMAJOR.MINOR.PATCH[-PRERELEASE]\n' "$0"
  printf 'Example: %s v0.3.0\n' "$0"
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

command -v git >/dev/null 2>&1 || die "git is required"
command -v go >/dev/null 2>&1 || die "go is required"
command -v gh >/dev/null 2>&1 || die "GitHub CLI (gh) is required"

[[ $# -eq 1 ]] || { usage; exit 1; }

version="$1"
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || \
  die "version must look like v0.3.0 or v0.3.0-rc.1"

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)" || die "not inside a Git repository"
cd "$repo_root"

[[ -z "$(git status --porcelain)" ]] || die "working tree is not clean; commit or stash changes first"
[[ "$(git branch --show-current)" == "main" ]] || die "releases must be created from main"
git rev-parse --verify "$version" >/dev/null 2>&1 && die "tag $version already exists locally"
git ls-remote --exit-code --tags origin "refs/tags/$version" >/dev/null 2>&1 && \
  die "tag $version already exists on origin"

printf 'Fetching origin and tags...\n'
git fetch origin main --tags

local_head="$(git rev-parse HEAD)"
remote_head="$(git rev-parse origin/main)"
[[ "$local_head" == "$remote_head" ]] || die "local main must match origin/main before release"

previous_tag="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
if [[ -n "$previous_tag" ]]; then
  range="$previous_tag..HEAD"
else
  range="HEAD"
fi

[[ -n "$(git log --format='%H' "$range")" ]] || die "no commits found since ${previous_tag:-repository creation}"

printf 'Running release checks...\n'
go test ./...
go vet ./...
build_output="$(mktemp "${TMPDIR:-/tmp}/terraviz-release-build.XXXXXX")"
notes_file="$(mktemp "${TMPDIR:-/tmp}/terraviz-release-notes.XXXXXX.md")"
trap 'rm -f "$build_output" "$notes_file"' EXIT
go build -o "$build_output" ./cmd/main.go

append_section() {
  local heading="$1"
  local pattern="$2"
  local entries
  entries="$(git log "$range" --format='%s|%h' | awk -F'|' -v pattern="$pattern" '
    BEGIN { IGNORECASE = 1 }
    $1 ~ pattern {
      subject = $1
      sub(/^[a-z]+(\([^)]*\))?!?:[[:space:]]*/, "", subject)
      printf "- %s (`%s`)\n", subject, $2
    }
  ')"
  if [[ -n "$entries" ]]; then
    printf '\n## %s\n\n%s\n' "$heading" "$entries" >> "$notes_file"
  fi
}

{
  printf '# TerraViz %s\n' "$version"
  printf '\nRelease generated from `%s` at commit `%s`.\n' "${previous_tag:-initial commit}" "$(git rev-parse --short HEAD)"
} > "$notes_file"

append_section "Features" '^(feat)(\([^)]*\))?!?:'
append_section "Fixes" '^(fix)(\([^)]*\))?!?:'
append_section "Performance" '^(perf)(\([^)]*\))?!?:'
append_section "Documentation" '^(docs)(\([^)]*\))?!?:'
append_section "Maintenance" '^(refactor|build|ci|chore|test)(\([^)]*\))?!?:'

other_entries="$(git log "$range" --format='%s|%h' | awk -F'|' '
  BEGIN { IGNORECASE = 1 }
  $1 !~ /^(feat|fix|perf|docs|refactor|build|ci|chore|test)(\([^)]*\))?!?:/ {
    printf "- %s (`%s`)\n", $1, $2
  }
')"
if [[ -n "$other_entries" ]]; then
  printf '\n## Other Changes\n\n%s\n' "$other_entries" >> "$notes_file"
fi

contributors="$(git log "$range" --format='%aN' | sort -fu | sed 's/^/- /')"
if [[ -n "$contributors" ]]; then
  printf '\n## Contributors\n\n%s\n' "$contributors" >> "$notes_file"
fi

repository_url="$(gh repo view --json url --jq '.url')"
if [[ -n "$previous_tag" ]]; then
  printf '\n**Full Changelog:** %s/compare/%s...%s\n' "$repository_url" "$previous_tag" "$version" >> "$notes_file"
fi

printf '\nGenerated release notes:\n\n'
command cat "$notes_file"

if [[ -n "${EDITOR:-}" ]]; then
  printf '\nOpening notes in %s...\n' "$EDITOR"
  "$EDITOR" "$notes_file"
fi

printf '\nPublish %s and trigger the GitHub release workflow? [y/N] ' "$version"
read -r answer
[[ "$answer" =~ ^[Yy]$ ]] || die "release cancelled"

git tag -a "$version" -F "$notes_file"
if ! git push origin "$version"; then
  git tag -d "$version" >/dev/null
  die "tag push failed; removed local tag $version"
fi

printf 'Waiting for the release workflow to start...\n'
run_id=""
for _ in {1..30}; do
  run_id="$(gh run list --workflow release.yml --event push --limit 20 \
    --json databaseId,headBranch --jq ".[] | select(.headBranch == \"$version\") | .databaseId" | sed -n '1p')"
  [[ -n "$run_id" ]] && break
  sleep 2
done

if [[ -z "$run_id" ]]; then
  printf 'Tag pushed, but the workflow was not found yet. Check %s/actions manually.\n' "$repository_url"
  exit 0
fi

if ! gh run watch "$run_id" --exit-status; then
  die "release workflow failed: $repository_url/actions/runs/$run_id"
fi

printf 'Applying curated release notes...\n'
gh release edit "$version" --notes-file "$notes_file"

printf 'Published %s/releases/tag/%s\n' "$repository_url" "$version"
