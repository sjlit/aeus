#!/usr/bin/env bash
# 检测自上次发布以来内容发生变更的 Go 模块。
# 归属规则:变更文件向上找最近的含 go.mod 的祖先目录,根模块为 "."。
# 基线规则:每个模块以它自己的最后一个 tag 为基线独立 diff。
#   不使用全局最近 tag——交错打 tag(热修复、手动 tag、发布中断残留)会把全局
#   基线推过某些模块尚未发布的变更,导致这些变更被静默吞掉、永不发版。
# 输出:每行一个模块路径(根模块为 "."),无变更时无输出。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# 根级非模块文件/目录:变更不计入任何模块。
# scripts/ 故意不在排除列表里——脚本本身的变更计入根模块,以触发根版本号 bump。
ROOT_EXCLUDE=(README.md Makefile .gitignore go.work.example go.work.sum docs .claude .vscode .trae .worktrees)

# 模块列表:子模块 + 根模块
submods="$(find . -name go.mod -not -path './go.mod' | sed 's|^\./||; s|/go\.mod$||' | sort)"

# 模块最后一个 tag;该模块从未打 tag 时输出空
last_tag() {
  local mod="$1" pat
  if [ "$mod" = "." ]; then
    pat='v[0-9]*'
  else
    pat="$mod/v[0-9]*"
  fi
  git tag -l "$pat" | sort -V | tail -1
}

# base..HEAD 区间内模块是否有内容变更;base 为空(从未打 tag)视为有变更
changed_since() {
  local base="$1"
  shift
  [ -z "$base" ] && return 0
  [ -n "$(git diff --name-only "$base"..HEAD -- "$@")" ]
}

out=""
for mod in $submods; do
  if changed_since "$(last_tag "$mod")" "$mod"; then
    out+="$mod"$'\n'
  fi
done

# 根模块:diff 限定在根级,排除子模块与根级非模块文件
paths=(.)
for m in $submods; do
  paths+=(":(exclude)$m")
done
for x in "${ROOT_EXCLUDE[@]}"; do
  paths+=(":(exclude)$x")
done
if changed_since "$(last_tag '.')" "${paths[@]}"; then
  out+="."$'\n'
fi

printf '%s' "$out"
