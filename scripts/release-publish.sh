#!/usr/bin/env bash
# Publish a staged release created by `make release-staged`.
#
# The root tag (and therefore the release commit) must be pushed before
# submodules can run `GOWORK=off go mod tidy`: Go resolves
# github.com/sjlit/aeus from the remote, not from the local tag. This
# script pushes the branch/root tag, syncs each submodule against the
# freshly pushed tags, commits the go.sum update, and only then tags the
# submodule — so every module tag is self-contained.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

STATE_FILE="$ROOT/.git/aeus-release-state"

fail() {
  echo "error: $*" >&2
  exit 1
}

if [ ! -f "$STATE_FILE" ]; then
  fail "missing $STATE_FILE; run make release-staged first"
fi
if [ -n "$(git status --porcelain)" ]; then
  fail "工作区有未提交的改动,请先 commit 或 stash"
fi
branch="$(git branch --show-current)"
if [ -z "$branch" ]; then
  fail "当前处于 detached HEAD,请在分支上执行发布"
fi

root_ver=""
state_branch=""
mods=()
while IFS='=' read -r k v; do
  [ -z "$k" ] && continue
  case "$k" in
    branch) state_branch="$v" ;;
    root) root_ver="$v" ;;
    *) mods+=("$k=$v") ;;
  esac
done < "$STATE_FILE"

if [ -n "$state_branch" ] && [ "$state_branch" != "$branch" ]; then
  fail "staged release belongs to branch $state_branch, current branch is $branch"
fi

wait_for_tag() {
  local tag="$1" i=0
  while [ "$i" -lt 30 ]; do
    if git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 2
  done
  fail "tag $tag 未在 origin 上出现(等待 60s 超时)"
}

push_tag() {
  local tag="$1"
  git push origin "refs/tags/$tag:refs/tags/$tag"
  wait_for_tag "$tag"
}

echo "==> push ${branch}"
git push origin "$branch"

if [ -n "$root_ver" ]; then
  echo "==> push root tag ${root_ver}"
  push_tag "$root_ver"
fi

# Non-admin modules only depend on the root module (transport/http is the
# exception, but admin is the only module that depends on transport/http).
# Process admin last so it can resolve the new transport/http tag.
ordered=()
if [ "${#mods[@]}" -gt 0 ]; then
  for pair in "${mods[@]}"; do
    [ "${pair%%=*}" = "admin" ] || ordered+=("$pair")
  done
  for pair in "${mods[@]}"; do
    [ "${pair%%=*}" = "admin" ] && ordered+=("$pair")
  done
fi

if [ "${#ordered[@]}" -gt 0 ]; then
  for pair in "${ordered[@]}"; do
    mod="${pair%%=*}"
    ver="${pair#*=}"
    [ -f "$mod/go.mod" ] || fail "missing $mod/go.mod"
    tag="$mod/$ver"
    if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
      fail "tag $tag 已存在"
    fi

    echo "==> sync $mod for $tag"
    (cd "$mod" && GOWORK=off go mod tidy)
    if [ -n "$(git status --porcelain -- "$mod")" ]; then
      git add "$mod"
      git commit -m "chore: sync $mod for ${ver}"
    fi

    git tag "$tag"
    git push origin "$branch"
    push_tag "$tag"
  done
fi

git push origin "$branch"
rm -f "$STATE_FILE"
echo
echo "Published tags:"
if [ -n "$root_ver" ]; then
  echo "  ${root_ver}"
fi
if [ "${#mods[@]}" -gt 0 ]; then
  for pair in "${mods[@]}"; do
    echo "  ${pair%%=*}/${pair#*=}"
  done
fi
