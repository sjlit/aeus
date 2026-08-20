<script setup lang="ts">
// 与 router/index.ts 里菜单路由 meta.componentName (= deriveComponentName('@/views/system/sys_role/Index.vue')) 对齐。
// 给 <keep-alive :include> 一个稳定可匹配的组件名,防止异步组件包装层丢失 name 导致视图不缓存。
defineOptions({ name: 'SystemSysRoles' })

import { getModelLabel, SchemaViewer } from '@sjlit/rest-ui'
import { usePageTitle } from '@/composables/usePageTitle'

// 页面标题跟随菜单(route meta.title 由路由注册时从菜单写入),
// 避免 SchemaViewer 回落到英文表名。
const title = usePageTitle('sys_roles')

/** 角色列表的 name 列渲染为跳到权限配置页的链接。 */
function rolePermPath(key: string): string {
  return '/system/sys_role/perm/' + key
}
</script>

<!--
  注意:不要传空的 #gridview 插槽——插槽一旦提供就会替换默认表格渲染,
  空模板等于把整张表藏掉。这里只在 name 列渲染 router-link,
  其他列需自行实测确认(详见设计稿 §9.3 风险提示)。
-->
<template>
  <SchemaViewer module="system" table="sys_roles" :title="title">
    <template #gridview="{ schema, model }">
      <span v-if="schema.column == 'name'">
        <router-link :to="rolePermPath(getModelLabel(model, 'key'))">
          {{ getModelLabel(model, 'name') }}
        </router-link>
      </span>
    </template>
  </SchemaViewer>
</template>
