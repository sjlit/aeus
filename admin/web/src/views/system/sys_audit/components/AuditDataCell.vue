<script setup lang="ts">
import { computed } from 'vue'

/**
 * 审计 data 字段渲染。
 *
 * data 是 JSON 字符串，结构来自 admin/audit.go 的 auditData()：
 *   - 新建/更新：[{ column, label, previous, current }]
 *   - 删除：{ id: ... }
 * 新建时 previous 为 null。
 */
const props = defineProps<{
  value: unknown
  /** 行级 action，用于判定是否新建（create）。列表请求下调用方需先拆掉 {label,value} 包装。 */
  action?: unknown
}>()

interface DiffChange {
  label: string
  previous: unknown
  current: unknown
}

type ParsedData =
  | { kind: 'changes'; changes: DiffChange[] }
  | { kind: 'delete'; id: unknown }
  | null

const rawText = computed(() => {
  const value = props.value
  if (value === null || value === undefined) return ''
  return typeof value === 'string' ? value : String(value)
})

const parsed = computed<ParsedData>(() => {
  const text = rawText.value.trim()
  if (!text) return null

  let decoded: unknown
  try {
    decoded = JSON.parse(text)
  } catch {
    return null
  }

  if (Array.isArray(decoded)) {
    return {
      kind: 'changes',
      changes: decoded.map((item) => {
        const row =
          item && typeof item === 'object' ? (item as Record<string, unknown>) : {}
        return {
          label: String(row.label ?? row.column ?? ''),
          previous: row.previous,
          current: row.current,
        }
      }),
    }
  }

  if (decoded && typeof decoded === 'object' && 'id' in decoded) {
    return { kind: 'delete', id: (decoded as Record<string, unknown>).id }
  }

  return null
})

const isParsed = computed(() => parsed.value !== null)
const isDelete = computed(() => parsed.value?.kind === 'delete')
const deletedId = computed(() =>
  parsed.value?.kind === 'delete' ? parsed.value.id : '',
)

const changes = computed<DiffChange[]>(() => {
  const value = parsed.value
  return value?.kind === 'changes' ? value.changes : []
})

/** 单元格里最多显示几条，其余通过 popover 展开。 */
const MAX_INLINE = 2
const inlineChanges = computed(() => changes.value.slice(0, MAX_INLINE))
const hiddenCount = computed(() => Math.max(0, changes.value.length - MAX_INLINE))

const hasAction = computed(
  () => props.action !== undefined && props.action !== null && props.action !== '',
)

const isCreateAction = computed(() => {
  const action = String(props.action ?? '').trim()
  // 兼容后端偶发返回枚举 label 的情况（create 对应「新建」）。
  return action === 'create' || action === '新建'
})

function isCreate(change: DiffChange): boolean {
  // action 存在时以行级标记为准，避免「更新时字段被清空」导致 previous=null 被误判为新增。
  if (hasAction.value) return isCreateAction.value
  return change.previous === null || change.previous === undefined
}

function displayValue(value: unknown): string {
  if (value === null || value === undefined) return '—'
  if (typeof value === 'string') return value === '' ? '""' : value
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value)
    } catch {
      return String(value)
    }
  }
  return String(value)
}
</script>

<template>
  <!-- 解析失败：回退显示原文，绝不把原始 JSON 串弄丢 -->
  <span v-if="!isParsed" class="audit-data-cell__raw">{{ rawText }}</span>

  <!-- 删除：{ id } -->
  <el-tag v-else-if="isDelete" type="danger" size="small" effect="light">
    删除记录 id={{ displayValue(deletedId) }}
  </el-tag>

  <!-- 变更数组：[{ column, label, previous, current }] -->
  <el-popover
    v-else
    placement="top"
    trigger="click"
    :width="420"
    popper-class="audit-data-popover"
  >
    <template #reference>
      <span class="audit-data-cell__summary">
        <template v-for="(change, index) in inlineChanges" :key="index">
          <span v-if="index > 0" class="audit-data-cell__sep"> · </span>
          <span class="audit-data-cell__item">
            <span class="audit-data-cell__label">{{ change.label }}:</span>
            <template v-if="isCreate(change)">
              <span class="audit-data-cell__plus">+</span>
              <span class="audit-data-cell__new">{{ displayValue(change.current) }}</span>
            </template>
            <template v-else>
              <del class="audit-data-cell__old">{{ displayValue(change.previous) }}</del>
              <span class="audit-data-cell__arrow">→</span>
              <span class="audit-data-cell__new">{{ displayValue(change.current) }}</span>
            </template>
          </span>
        </template>
        <span v-if="changes.length === 0" class="audit-data-cell__empty">无变更</span>
        <span v-if="hiddenCount > 0" class="audit-data-cell__more">+{{ hiddenCount }}</span>
      </span>
    </template>

    <div class="audit-diff">
      <table class="audit-diff__table">
        <thead>
          <tr>
            <th>字段</th>
            <th v-if="!isCreateAction">变更前</th>
            <th>{{ isCreateAction ? '新增值' : '变更后' }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(change, index) in changes" :key="index">
            <td class="audit-diff__field">{{ change.label }}</td>
            <td v-if="!isCreateAction">
              <del class="audit-diff__old">{{ displayValue(change.previous) }}</del>
            </td>
            <td>
              <span class="audit-diff__new">{{ displayValue(change.current) }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </el-popover>
</template>

<style scoped>
.audit-data-cell__raw,
.audit-data-cell__summary {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}

.audit-data-cell__raw {
  font-family: var(--mono);
  font-size: 12px;
  color: var(--ink-3);
}

.audit-data-cell__summary {
  cursor: pointer;
  font-size: 12px;
  line-height: 1.6;
}

.audit-data-cell__item {
  white-space: nowrap;
}

.audit-data-cell__label {
  color: var(--ink-2);
  font-weight: 600;
}

.audit-data-cell__sep {
  color: var(--ink-3);
  padding: 0 2px;
}

.audit-data-cell__old {
  color: var(--el-color-danger);
  text-decoration: line-through;
}

.audit-data-cell__new {
  color: var(--el-color-success);
  font-family: var(--mono);
}

.audit-data-cell__plus {
  color: var(--el-color-success);
  font-weight: 700;
  margin-right: 2px;
}

.audit-data-cell__arrow {
  color: var(--ink-3);
  margin: 0 3px;
}

.audit-data-cell__more {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  border-radius: 8px;
  padding: 0 6px;
  margin-left: 4px;
}

.audit-data-cell__empty {
  color: var(--ink-3);
}

.audit-diff__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.audit-diff__table th,
.audit-diff__table td {
  text-align: left;
  vertical-align: top;
  padding: 6px 8px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  word-break: break-all;
}

.audit-diff__table th {
  color: var(--ink-3);
  font-weight: 600;
}

.audit-diff__field {
  color: var(--ink-2);
  font-weight: 600;
  white-space: nowrap;
}

.audit-diff__old {
  color: var(--el-color-danger);
  text-decoration: line-through;
  font-family: var(--mono);
}

.audit-diff__new {
  color: var(--el-color-success);
  font-family: var(--mono);
}
</style>
