#!/usr/bin/env bash
# 将指定子模块对根模块的 require 更新为指定版本。
# 用法:./scripts/bump-version.sh <root-version> [module-dir ...]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

NEW_VERSION=$1
shift

MAIN_MODULE="$(sed -n 's/^module //p' go.mod)"

for mod in "$@"; do
  (cd "$mod" && go mod edit -require="${MAIN_MODULE}@${NEW_VERSION}")
done

echo "Updated require ${MAIN_MODULE} ${NEW_VERSION} in: $*"
