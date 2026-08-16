#!/usr/bin/env bash
# 按需发布:仅给内容发生变更的 Go 模块升版本并打标签。
#
# 用法(环境变量):
#   make release                                  # 全部自动 patch+1
#   make release VERSION=v1.1.0                   # 根模块升到指定版本
#   make release MODULES="transport/http=v1.2.0"  # 指定子模块版本
#   make release DRY_RUN=1                        # 只打印,不执行
#
# 兼容性:仅使用 bash 3.2 语法(macOS 自带 /bin/bash),不用关联数组(declare -A)
# 与 readarray(均为 bash 4+ 特性);模块→版本映射用 "mod=ver" 多行文本 + awk 实现。
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

# 从 "mod=ver" 多行文本中按模块名取版本;未命中输出空。
ver_of() {
  awk -v m="$1" -F= '$1 == m { print $2; exit }' <<<"$2"
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

# --- 解析 MODULES="path=v1.2.0 ..." 覆盖,存为 "mod=ver" 多行文本 ---
OVERRIDE=""
for pair in $MODULES; do
  mod="${pair%%=*}"
  ver="${pair#*=}"
  [ "$mod" != "$pair" ] || fail "MODULES 项 \"$pair\" 缺少 =版本,应为 path=v1.2.0"
  [[ "$ver" =~ $SEMVER_RE ]] || fail "MODULES 版本 \"$ver\" 不是合法 semver(应为 vX.Y.Z)"
  grep -Fqx "$mod" <<<"$KNOWN_MODS" || fail "MODULES 路径 \"$mod\" 不是已知模块"
  OVERRIDE="${OVERRIDE}${mod}=${ver}"$'\n'
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

# --- 检测变更模块并计算版本,存为 "mod=ver" 多行文本 ---
changed="$(./scripts/detect-changed-modules.sh)"

RELEASE=""
for mod in $changed; do
  [ -z "$mod" ] && continue
  ver="$(ver_of "$mod" "$OVERRIDE")"
  if [ -z "$ver" ]; then
    ver="$(next_version "$mod")"
  fi
  RELEASE="${RELEASE}${mod}=${ver}"$'\n'
done

if [ -z "$RELEASE" ]; then
  echo "nothing to release: 自上次发布以来没有模块内容变更" >&2
  exit 1
fi

# 根模块版本:VERSION 优先于 MODULES 覆盖;根模块无变更时忽略 VERSION
root_ver="$(ver_of "." "$RELEASE")"
if [ -n "$root_ver" ] && [ -n "$VERSION" ]; then
  RELEASE="$(printf '%s\n' "$RELEASE" | awk -F= -v v="$VERSION" '$1 == "." { print $1"="v; next } { print }')"
elif [ -z "$root_ver" ] && [ -n "$VERSION" ]; then
  echo "note: VERSION=$VERSION 指定了根模块版本,但根模块本次无变更,已忽略" >&2
fi
root_ver="$(ver_of "." "$RELEASE")"

# 排序后的模块名列表(索引数组)
keys=()
while IFS= read -r k; do
  [ -n "$k" ] && keys+=("$k")
done < <(printf '%s\n' "$RELEASE" | sed 's|=.*$||' | sort)

# --- 预检:目标 tag 不能已存在(防止二次发布把版本跳过) ---
for k in "${keys[@]}"; do
  ver="$(ver_of "$k" "$RELEASE")"
  if [ "$k" = "." ]; then
    t="$ver"
  else
    t="$k/$ver"
  fi
  if git rev-parse -q --verify "refs/tags/$t" >/dev/null; then
    fail "tag $t 已存在,请确认版本号(如需重发请先删掉该 tag)"
  fi
done

# --- 组装提交信息 ---
parts=()
for k in "${keys[@]}"; do
  ver="$(ver_of "$k" "$RELEASE")"
  if [ "$k" = "." ]; then
    parts+=("root ${ver}")
  else
    parts+=("${k} ${ver}")
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
  if [ "$COMMITTED" = "1" ]; then
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
  if [ -n "$root_ver" ]; then
    root_version="$root_ver"
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
  ver="$(ver_of "$k" "$RELEASE")"
  if [ "$k" = "." ]; then
    git tag "$ver"
    CREATED_TAGS+=("$ver")
    echo "tagged: $ver"
  else
    git tag "$k/$ver"
    CREATED_TAGS+=("$k/$ver")
    echo "tagged: $k/$ver"
  fi
done

trap - ERR
echo "Done. 推送: git push origin ${branch} && git push origin --tags"
