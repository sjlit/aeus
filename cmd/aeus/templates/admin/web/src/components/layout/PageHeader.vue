<script setup lang="ts">
/**
 * PageHeader · 与 rest-ui 的 .schema-page-header 对齐的手写页页头。
 * 左侧标题/说明,右侧主操作;不画卡片,让页面内容区保持一个统一的顶部节奏。
 */
withDefaults(defineProps<{
  title: string
  subtitle?: string
}>(), {
  subtitle: '',
})
</script>

<template>
  <header class="page-header">
    <div class="page-header__main">
      <h1 class="page-header__title">{{ title }}</h1>
      <p v-if="subtitle || $slots.description" class="page-header__desc">
        <slot name="description">{{ subtitle }}</slot>
      </p>
    </div>

    <div v-if="$slots.actions" class="page-header__actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  padding: 4px 2px 0;
}

.page-header__main {
  min-width: 0;
}

.page-header__title {
  margin: 0;
  font-family: var(--display);
  font-size: var(--fs-lg);
  font-weight: 600;
  line-height: 1.25;
  letter-spacing: -0.01em;
  color: var(--ink);
}

.page-header__desc {
  margin: 6px 0 0;
  font-size: var(--fs-sm);
  line-height: 1.45;
  color: var(--ink-2);
}

.page-header__actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 768px) {
  .page-header__actions {
    width: 100%;
  }
}
</style>
