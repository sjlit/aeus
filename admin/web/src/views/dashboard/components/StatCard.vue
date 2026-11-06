<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { Component } from 'vue'

const props = withDefaults(defineProps<{
  label: string
  value: number | string
  hint?: string
  icon?: Component
  /** 数字下方的辅助信息语气;up/down 只影响颜色,不影响布局。 */
  tone?: 'default' | 'up' | 'down'
  /** 有权限对应列表页时传路由,整张卡变成可点击入口。 */
  to?: string
  loading?: boolean
}>(), {
  hint: '',
  tone: 'default',
  to: '',
  loading: false,
})

const formattedValue = computed(() =>
  typeof props.value === 'number'
    ? props.value.toLocaleString('zh-CN')
    : props.value,
)

const tag = computed(() => (props.to ? RouterLink : 'div'))
const tagProps = computed(() => (props.to ? { to: props.to } : {}))
</script>

<template>
  <component
    :is="tag"
    v-bind="tagProps"
    class="stat-card"
    :class="{ 'stat-card--link': to }"
    :aria-label="to ? `${label}:${formattedValue},打开列表` : undefined"
  >
    <div class="stat-card__head">
      <span class="stat-card__label">{{ label }}</span>
      <span v-if="icon" class="stat-card__icon" aria-hidden="true">
        <el-icon><component :is="icon" /></el-icon>
      </span>
    </div>

    <div v-if="loading" class="stat-card__loading skeleton" aria-hidden="true"></div>
    <p v-else class="stat-card__value">{{ formattedValue }}</p>
    <p v-if="hint" class="stat-card__hint" :class="`stat-card__hint--${tone}`">
      {{ hint }}
    </p>
  </component>
</template>

<style scoped>
.stat-card {
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 18px 20px 16px;
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: 0 10px 30px var(--shadow-soft);
  color: inherit;
  text-decoration: none;
  transition:
    border-color 0.16s ease,
    box-shadow 0.16s ease,
    transform 0.16s ease;
}

.stat-card--link:hover {
  border-color: rgba(47, 125, 99, 0.28);
  box-shadow: 0 14px 34px rgba(30, 90, 90, 0.12);
  transform: translateY(-1px);
}

.stat-card--link:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 2px;
}

.stat-card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.stat-card__label {
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink-2);
}

.stat-card__icon {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  background: var(--menu-active-tint-soft);
  color: var(--acc-mint-deep);
  font-size: var(--fs-md);
}

.stat-card__value {
  margin-top: 10px;
  font-family: var(--display);
  font-size: var(--fs-2xl);
  font-weight: 700;
  line-height: 1.1;
  letter-spacing: -0.03em;
  font-variant-numeric: tabular-nums;
  color: var(--ink);
}

.stat-card__hint {
  margin-top: 8px;
  font-size: var(--fs-xs);
  line-height: 1.4;
  color: var(--ink-3);
}

.stat-card__hint--up {
  color: var(--el-color-success);
}

.stat-card__hint--down {
  color: var(--el-color-danger);
}

.stat-card__loading {
  width: 96px;
  height: 34px;
  margin-top: 10px;
  border-radius: 8px;
}
</style>
