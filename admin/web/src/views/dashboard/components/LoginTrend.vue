<script setup lang="ts">
import { computed } from 'vue'
import { TrendCharts } from '@element-plus/icons-vue'
import type { DashboardTrendPoint } from '@/api/dashboard'

const props = defineProps<{
  points: DashboardTrendPoint[]
  today: number
  yesterday: number
}>()

const W = 620
const H = 190
const PAD = { top: 16, right: 16, bottom: 28, left: 34 }

const hasData = computed(() => props.points.some((point) => point.count > 0))

const chart = computed(() => {
  const points = props.points
  if (points.length === 0) return null
  const max = Math.max(...points.map((point) => point.count), 1)
  const innerW = W - PAD.left - PAD.right
  const innerH = H - PAD.top - PAD.bottom
  const step = points.length > 1 ? innerW / (points.length - 1) : 0
  const coords = points.map((point, index) => {
    const x = PAD.left + index * step
    const y = PAD.top + innerH - (point.count / max) * innerH
    return { ...point, x, y }
  })
  const line = coords
    .map((point, index) => `${index === 0 ? 'M' : 'L'}${point.x.toFixed(1)} ${point.y.toFixed(1)}`)
    .join(' ')
  const baseline = PAD.top + innerH
  const area = [
    `M${coords[0].x.toFixed(1)} ${baseline}`,
    ...coords.map((point) => `L${point.x.toFixed(1)} ${point.y.toFixed(1)}`),
    `L${coords[coords.length - 1].x.toFixed(1)} ${baseline}`,
    'Z',
  ].join(' ')
  const yTicks = [0, 0.5, 1].map((fraction) => ({
    y: PAD.top + innerH - fraction * innerH,
    label: Math.round(fraction * max),
  }))
  const xLabels = coords.map((point) => ({
    x: point.x,
    label: point.date.slice(5),
  }))
  return { coords, line, area, yTicks, xLabels, baseline }
})

const delta = computed(() => props.today - props.yesterday)
const deltaText = computed(() => {
  if (delta.value === 0) return '与昨日持平'
  return `较昨日 ${delta.value > 0 ? '+' : ''}${delta.value}`
})

const summary = computed(() => {
  const total = props.points.reduce((sum, point) => sum + point.count, 0)
  return `近 7 天登录 ${total} 次,今日 ${props.today} 次,昨日 ${props.yesterday} 次`
})
</script>

<template>
  <section class="surface-card trend-card" aria-labelledby="trend-title">
    <header class="panel-head">
      <div>
        <h2 id="trend-title" class="panel-title">登录趋势</h2>
        <p class="panel-sub">近 7 天 · 今日 {{ today }} 次</p>
      </div>
      <span class="trend-delta" :class="{ 'is-down': delta < 0 }">{{ deltaText }}</span>
    </header>

    <div v-if="hasData && chart" class="chart-wrap">
      <svg
        class="chart"
        :viewBox="`0 0 ${W} ${H}`"
        role="img"
        :aria-label="summary"
      >
        <defs>
          <linearGradient id="dashboard-login-area" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#2f7d63" stop-opacity="0.18" />
            <stop offset="100%" stop-color="#2f7d63" stop-opacity="0" />
          </linearGradient>
        </defs>

        <g class="chart-grid" aria-hidden="true">
          <line
            v-for="(tick, index) in chart.yTicks"
            :key="`grid-${index}`"
            :x1="PAD.left"
            :x2="W - PAD.right"
            :y1="tick.y"
            :y2="tick.y"
          />
          <text
            v-for="(tick, index) in chart.yTicks"
            :key="`label-${index}`"
            :x="PAD.left - 8"
            :y="tick.y + 4"
            text-anchor="end"
          >{{ tick.label }}</text>
        </g>

        <path :d="chart.area" fill="url(#dashboard-login-area)" />
        <path class="chart-line" :d="chart.line" fill="none" />

        <g class="chart-points">
          <circle
            v-for="point in chart.coords.filter((item) => item.count > 0)"
            :key="point.date"
            :cx="point.x"
            :cy="point.y"
            r="3.5"
          >
            <title>{{ point.date }}:{{ point.count }} 次登录</title>
          </circle>
        </g>

        <g class="chart-x" aria-hidden="true">
          <text
            v-for="label in chart.xLabels"
            :key="label.label"
            :x="label.x"
            :y="H - 8"
            text-anchor="middle"
          >{{ label.label }}</text>
        </g>
      </svg>
    </div>

    <div v-else class="trend-empty">
      <span class="trend-empty-icon" aria-hidden="true">
        <el-icon><TrendCharts /></el-icon>
      </span>
      <p>暂无登录记录</p>
      <span>接入登录日志后,这里会显示 7 天趋势。</span>
    </div>
  </section>
</template>

<style scoped>
.trend-card {
  padding: 20px 22px 14px;
}

.trend-delta {
  flex: 0 0 auto;
  padding: 4px 8px;
  border-radius: 999px;
  background: rgba(46, 125, 50, 0.08);
  color: var(--el-color-success);
  font-size: var(--fs-xs);
  font-weight: 500;
}

.trend-delta.is-down {
  background: rgba(192, 57, 43, 0.08);
  color: var(--el-color-danger);
}

.chart-wrap {
  margin-top: 8px;
}

.chart {
  display: block;
  width: 100%;
  height: auto;
  overflow: visible;
}

.chart text {
  font-family: var(--mono);
  font-size: var(--fs-2xs);
  fill: var(--ink-3);
}

.chart-grid line {
  stroke: var(--line-soft);
  stroke-width: 1;
}

.chart-line {
  stroke: var(--acc-mint-deep);
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.chart-points circle {
  fill: var(--surface);
  stroke: var(--acc-mint-deep);
  stroke-width: 2;
}

.chart-x text {
  font-size: var(--fs-2xs);
}

.trend-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 190px;
  padding: 16px;
  text-align: center;
}

.trend-empty-icon {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 13px;
  background: var(--surface-soft);
  color: var(--acc-mint-deep);
  font-size: 21px;
}

.trend-empty p {
  margin: 12px 0 0;
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink);
}

.trend-empty span {
  margin-top: 4px;
  font-size: var(--fs-xs);
  color: var(--ink-3);
}
</style>
