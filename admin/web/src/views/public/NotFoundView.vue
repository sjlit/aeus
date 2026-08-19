<script setup lang="ts">
// 与 router/index.ts 里 NotFoundView 路由 meta.componentName (= deriveComponentName('@/views/public/NotFoundView.vue')) 对齐。
// 给 <keep-alive :include> 一个稳定可匹配的组件名,防止异步组件包装层丢失 name 导致视图不缓存。
defineOptions({ name: 'PublicNotFoundView' })

import { useRouter } from 'vue-router'
const router = useRouter()
import { usePageTitle } from '@/composables/usePageTitle'

const pageTitle = usePageTitle()

</script>

<template>
  <section class="glass panel notfound">
    <h1 class="text-gradient--peach">404</h1>
    <p><b>{{ pageTitle }}</b> 不存在。</p>
    <el-button type="primary" @click="router.push('/')">返回首页</el-button>
  </section>
</template>

<style scoped>
.notfound {
  max-width: 480px;
  margin: 48px auto;
  text-align: center;
}

.notfound h1 {
  font-family: var(--display);
  font-size: 64px;
  font-weight: 600;
  letter-spacing: -0.03em;
  margin-bottom: 12px;
}

.notfound p {
  font-size: 13px;
  color: var(--ink-2);
  margin-bottom: 20px;
}
</style>
