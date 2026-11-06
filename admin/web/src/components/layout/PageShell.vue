<script setup lang="ts">
/**
 * PageShell · 手写业务页的统一页面骨架。
 * 只负责纵向节奏与可选的内容宽度,不承载页头/卡片语义;
 * 列表、表单、详情页都从这里开始,避免每个页面各写一套 padding。
 */
withDefaults(defineProps<{
  /** 表单/详情类页面使用 1080px 居中版心。 */
  narrow?: boolean
}>(), {
  narrow: false,
})
</script>

<template>
  <div class="page-shell" :class="{ 'page-shell--narrow': narrow }">
    <slot />
  </div>
</template>

<style scoped>
.page-shell {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  min-height: 100%;
}

.page-shell--narrow {
  max-width: 1080px;
  margin: 0 auto;
}
</style>
