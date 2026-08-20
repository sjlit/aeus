<script setup lang="ts">
// 与 router/index.ts 里菜单路由 meta.componentName (= deriveComponentName('@/views/system/sys_permission/Index.vue')) 对齐。
// 给 <keep-alive :include> 一个稳定可匹配的组件名,防止异步组件包装层丢失 name 导致视图不缓存。
defineOptions({ name: 'SystemSysPermissions' })

import { SchemaViewer } from '@sjlit/rest-ui'
import { usePageTitle } from '@/composables/usePageTitle'

// 页面标题跟随菜单(route meta.title 由路由注册时从菜单写入),
// 避免 SchemaViewer 回落到英文表名。
const title = usePageTitle('sys_permissions')
</script>

<!--
  注意:不要传空的 #gridview 插槽——插槽一旦提供就会替换默认表格渲染,
  空模板等于把整张表藏掉。需要定制表格时,在插槽里给出完整实现。
-->
<template>
  <SchemaViewer module="system" table="sys_permissions" :title="title" />
</template>
