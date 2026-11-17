<script setup lang="ts">
// 与 router/index.ts 里菜单路由 meta.componentName (= deriveComponentName('@/views/system/sys_audit/Index.vue')) 对齐。
// 给 <keep-alive :include> 一个稳定可匹配的组件名,防止异步组件包装层丢失 name 导致视图不缓存。
defineOptions({ name: 'SystemSysAudits' })

import { getModelValue, unwrapCellValue, SchemaViewer } from '@sjlit/rest-ui'
import { usePageTitle } from '@/composables/usePageTitle'
import AuditDataCell from './components/AuditDataCell.vue'

// 页面标题跟随菜单(route meta.title 由路由注册时从菜单写入),
// 避免 SchemaViewer 回落到英文表名。
const title = usePageTitle('sys_audits')
</script>

<!--
  #gridview 是按单元格调用的插槽：只对 data 列做定制，其余列返回空内容，
  由 rest-ui 的内置 <Cell> 兜底渲染（Vue renderSlot 的 fallback 机制）。
-->
<template>
  <SchemaViewer module="system" table="sys_audits" :title="title" readonly>
    <template #gridview="{ schema, model }">
      <AuditDataCell
        v-if="schema.column === 'data'"
        :value="unwrapCellValue(getModelValue(model, 'data'))"
        :action="unwrapCellValue(getModelValue(model, 'action'))"
      />
    </template>
  </SchemaViewer>
</template>
