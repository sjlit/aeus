<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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

/* 轴标签的字号要恒定,不能跟着 viewBox 一起缩放。
   SVG 用 viewBox + width:100% 渲染时,整张图会等比缩放:
   桌面(宽 717px)缩放 1.16,声明的 11px 实际渲染成 12.7px;
   移动端(宽 316px)缩放 0.51,11px 实际只剩 5.6px —— 轴标签基本看不清。
   这里用 ResizeObserver 量出真实缩放比,反算出 viewBox 用户单位下的字号,
   让渲染像素恒定在 AXIS_FONT_PX。 */
const AXIS_FONT_PX = 11
const chartRef = ref<SVGSVGElement | null>(null)
let observer: ResizeObserver | null = null

function syncAxisFontSize() {
  const svg = chartRef.value
  if (!svg) return
  const rendered = svg.getBoundingClientRect().width
  if (rendered <= 0) return
  const scale = rendered / W
  // 字号写在 viewBox 用户单位里,渲染像素 = 用户单位 × 缩放比
  svg.style.setProperty('--chart-text-px', `${AXIS_FONT_PX / scale}px`)
}

onMounted(() => {
  const svg = chartRef.value
  if (!svg || typeof ResizeObserver === 'undefined') return
  observer = new ResizeObserver(syncAxisFontSize)
  observer.observe(svg)
  syncAxisFontSize()
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})

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

// 窗口长度由接口返回的点数决定,不写死 7。
// 后端目前固定返回 7 天,但前端把天数断言成常量的话,
// 后端一旦改成 14/30 天,文案就会静默说谎。
const windowDays = computed(() => props.points.length)

const summary = computed(() => {
  const total = props.points.reduce((sum, point) => sum + point.count, 0)
  return `近 ${windowDays.value} 天登录 ${total} 次,今日 ${props.today} 次,昨日 ${props.yesterday} 次`
})
</script>

<template>
  <section class="surface-card trend-card" aria-labelledby="trend-title">
    <header class="panel-head">
      <div>
        <h2 id="trend-title" class="panel-title">登录趋势</h2>
        <p class="panel-sub">近 {{ windowDays }} 天 · 今日 {{ today }} 次</p>
      </div>
      <span class="trend-delta" :class="{ 'is-down': delta < 0 }">{{ deltaText }}</span>
    </header>

    <div v-if="hasData && chart" class="chart-wrap">
      <svg
        ref="chartRef"
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
      <span>接入登录日志后,这里会显示 {{ windowDays }} 天趋势。</span>
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
  /* 底色取语义色的浅底派生色,不再硬编码 rgba 值 ——
     换语义色时不用再回头改这里。 */
  background: var(--el-color-success-light-9);
  color: var(--el-color-success);
  font-size: var(--fs-xs);
  font-weight: 500;
}

.trend-delta.is-down {
  background: var(--el-color-danger-light-9);
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
  /* 字号由 ResizeObserver 按真实缩放比反算(见上方 AXIS_FONT_PX),
     保证各视口下渲染字号恒定,不被 viewBox 缩放带走。 */
  font-size: var(--chart-text-px, var(--fs-2xs));
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
  font-size: var(--chart-text-px, var(--fs-2xs));
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
