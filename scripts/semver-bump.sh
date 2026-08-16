#!/usr/bin/env bash
# patch 号 +1。输入必须为严格 vX.Y.Z,否则拒绝。
set -euo pipefail

version=$1
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo "error: semver-bump: 非法版本 \"$version\"(应为 vX.Y.Z)" >&2
  exit 1
}

v=${version#v}
major=$(echo "$v" | cut -d. -f1)
minor=$(echo "$v" | cut -d. -f2)
patch=$(echo "$v" | cut -d. -f3)
patch=$((patch + 1))
echo "v${major}.${minor}.${patch}"
