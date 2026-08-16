#!/usr/bin/env bash
# 按需发布:仅给内容发生变更的 Go 模块升版本并打标签。
#
# 用法(环境变量):
#   make release                                  # 全部自动 patch+1
#   make release VERSION=v1.1.0                   # 根模块升到指定版本
#   make release MODULES="transport/http=v1.2.0"  # 指定子模块版本
#   make release DRY_RUN=1                        # 只打印,不执行
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-}"
MODULES="${MODULES:-}"
DRY_RUN="${DRY_RUN:-0}"

# 严格 semver:仅 vX.Y.Z(与 semver-bump.sh 的 patch+1 规则一致)
SEMVER_RE='^v[0-9]+\.[0-9]+\.[0-9]+$'

fail() {
  echo "error: $*" >&2
  exit 1
}

# --- 预检:干净工作区 + 在分支上。所有预检都在任何仓库变更之前,失败不留痕迹 ---
if [ -n "$(git status --porcelain)" ]; then
  fail "工作区有未提交的改动,请先 commit 或 stash"
fi
branch="$(git branch --show-current)"
if [ -z "$branch" ]; then
  fail "当前处于 detached HEAD,请在分支上执行发布"
fi

# --- 已知模块列表(与 detect-changed-modules.sh 的归属规则一致) ---
KNOWN_MODS="$( { find . -name go.mod -not -path './go.mod' | sed 's|^\./||; s|/go\.mod$||' | sort; echo .; } )"

# --- 解析 MODULES="path=v1.2.0 ..." 覆盖 ---
declare -A OVERRIDE=()
for pair in $MODULES; do
  mod="${pair%%=*}"
  ver="${pair#*=}"
  [ "$mod" != "$pair" ] || fail "MODULES 项 \"$pair\" 缺少 =版本,应为 path=v1.2.0"
  [[ "$ver" =~ $SEMVER_RE ]] || fail "MODULES 版本 \"$ver\" 不是合法 semver(应为 vX.Y.Z)"
  grep -Fqx "$mod" <<<"$KNOWN_MODS" || fail "MODULES 路径 \"$mod\" 不是已知模块"
  OVERRIDE["$mod"]="$ver"
done
if [ -n "$VERSION" ]; then
  [[ "$VERSION" =~ $SEMVER_RE ]] || fail "VERSION \"$VERSION\" 不是合法 semver(应为 vX.Y.Z)"
fi

# 模块下一版本:有覆盖用覆盖,否则自动 patch+1;无 tag 则 v1.0.0
next_version() {
  local pat last
  if [ "$1" = "." ]; then
    pat='v[0-9]*'
  else
    pat="$1/v[0-9]*"
  fi
  last="$(git tag -l "$pat" | sort -V | tail -1)"
  if [ -z "$last" ]; then
    echo "v1.0.0"
  else
    # 剥掉模块前缀(如 "transport/http/v1.0.0" → "v1.0.0")再 patch+1
    ./scripts/semver-bump.sh "${last##*/}"
  fi
}

# --- 检测变更模块并计算版本 ---
changed="$(./scripts/detect-changed-modules.sh)"

declare -A RELEASE=()
for mod in $changed; do
  [ -z "$mod" ] && continue
  if [ -n "${OVERRIDE[$mod]:-}" ]; then
    RELEASE["$mod"]="${OVERRIDE[$mod]}"
  else
    RELEASE["$mod"]="$(next_version "$mod")"
  fi
done

if [ "${#RELEASE[@]}" -eq 0 ]; then
  echo "nothing to release: 自上次发布以来没有模块内容变更" >&2
  exit 1
fi

# 根模块版本:VERSION 优先于 MODULES 覆盖;根模块无变更时忽略 VERSION
if [ -n "${RELEASE[.]:-}" ] && [ -n "$VERSION" ]; then
  RELEASE["."]="$VERSION"
elif [ -z "${RELEASE[.]:-}" ] && [ -n "$VERSION" ]; then
  echo "note: VERSION=$VERSION 指定了根模块版本,但根模块本次无变更,已忽略" >&2
fi

# 关联数组无序,先排序
readarray -t keys < <(printf '%s\n' "${!RELEASE[@]}" | sort)

# --- 预检:目标 tag 不能已存在(防止二次发布把版本跳过) ---
for k in "${keys[@]}"; do
  if [ "$k" = "." ]; then
    t="${RELEASE[$k]}"
  else
    t="$k/${RELEASE[$k]}"
  fi
  if git rev-parse -q --verify "refs/tags/$t" >/dev/null; then
    fail "tag $t 已存在,请确认版本号(如需重发请先删掉该 tag)"
  fi
done

# --- 组装提交信息 ---
parts=()
for k in "${keys[@]}"; do
  if [ "$k" = "." ]; then
    parts+=("root ${RELEASE[$k]}")
  else
    parts+=("${k} ${RELEASE[$k]}")
  fi
done
joined="$(IFS=,; echo "${parts[*]}")"
msg="chore: release ${joined//,/, }"

if [ "$DRY_RUN" = "1" ]; then
  echo "DRY RUN — 将发布:${msg#chore: release }"
  exit 0
fi

# --- 失败回滚:清除本次创建的 tag、回退本次创建的 commit ---
CREATED_TAGS=()
COMMITTED=0
cleanup() {
  local code=$?
  if [ "$COMMITTED" = 1 ]; then
    git reset --hard -q HEAD~1 || true
  fi
  if [ "${#CREATED_TAGS[@]}" -gt 0 ]; then
    git tag -d "${CREATED_TAGS[@]}" >/dev/null 2>&1 || true
  fi
  exit "$code"
}
trap cleanup ERR

# --- 更新本次发布子模块对根模块的 require ---
# 即便本次不发布根模块,子模块的 require 也必须对齐当前最新根 tag,
# 避免发布出去的子模块引用过期的根版本。
submods=()
for k in "${keys[@]}"; do
  [ "$k" != "." ] && submods+=("$k")
done
if [ "${#submods[@]}" -gt 0 ]; then
  # 优先用本次根发布的新版本;否则对齐当前最新根 tag
  if [ -n "${RELEASE[.]:-}" ]; then
    root_version="${RELEASE[.]}"
  else
    root_version="$(git tag -l 'v[0-9]*' | sort -V | tail -1)"
  fi
  if [ -n "$root_version" ]; then
    ./scripts/bump-version.sh "$root_version" "${submods[@]}"
  fi
fi

# --- 提交并打标签(无文件改动时跳过提交,直接对 HEAD 打标签) ---
git add -A
if [ -n "$(git diff --cached --name-only)" ]; then
  git commit -m "$msg"
  COMMITTED=1
fi

for k in "${keys[@]}"; do
  if [ "$k" = "." ]; then
    git tag "${RELEASE[$k]}"
    CREATED_TAGS+=("${RELEASE[$k]}")
    echo "tagged: ${RELEASE[$k]}"
  else
    git tag "$k/${RELEASE[$k]}"
    CREATED_TAGS+=("$k/${RELEASE[$k]}")
    echo "tagged: $k/${RELEASE[$k]}"
  fi
done

trap - ERR
echo "Done. 推送: git push origin ${branch} && git push origin --tags"
