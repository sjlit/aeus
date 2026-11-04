#!/usr/bin/env bash
# Sync submodule go.sum files after a release has been tagged and pushed.
#
# release.sh updates intra-repo requires (root / transport/http) but cannot
# compute go.sum entries for versions that do not exist on the remote yet.
# Run this after `git push origin --tags`, review the diff, and commit it.
#
# Usage:
#   ./scripts/sync-module-sums.sh
#   ./scripts/sync-module-sums.sh admin transport/http
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [ "$#" -gt 0 ]; then
  modules=("$@")
else
  modules=(admin transport/http transport/cli infra/telemetry/prometheus)
fi

for mod in "${modules[@]}"; do
  if [ ! -f "$mod/go.mod" ]; then
    echo "error: $mod/go.mod not found" >&2
    exit 1
  fi
  echo "==> sync $mod"
  (cd "$mod" && GOWORK=off go mod tidy)
done

echo
echo "Review and commit the go.sum changes:"
git status --short
