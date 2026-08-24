<script setup lang="ts">
// 与 router/index.ts 里这条静态路由的 meta.componentName (= deriveComponentName
// '@/views/dashboard/Index.vue') 对齐。keep-alive :include=tabs.cachedViews
// 要求稳定可匹配的 name,异步组件包装层一旦丢失 name 就会让缓存失效。
defineOptions({ name: 'DashboardIndex' })

import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { usePageTitle } from '@/composables/usePageTitle'

const title = usePageTitle('工作台')
const auth = useAuthStore()
const menu = useMenuStore()

/** 按时间段问候。 */
const greeting = computed(() => {
  const h = new Date().getHours()
  if (h < 6) return '夜深了'
  if (h < 12) return '早上好'
  if (h < 14) return '中午好'
  if (h < 18) return '下午好'
  return '晚上好'
})

/** 实时时钟(mono 字体,秒级跳动)。 */
const now = ref(new Date())
let timer: number | undefined
onMounted(() => {
  timer = window.setInterval(() => { now.value = new Date() }, 1000)
})
onUnmounted(() => { if (timer !== undefined) window.clearInterval(timer) })

const clock = computed(() =>
  now.value.toLocaleTimeString('zh-CN', { hour12: false }),
)
const dateLine = computed(() =>
  new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', weekday: 'long',
  }).format(now.value),
)

/** 指标读数:当前为占位,接入真实统计接口时把这份配置换成接口返回值即可。 */
interface StatItem {
  key: string
  label: string
  value: string
}

const stats: StatItem[] = [
  { key: 'users', label: 'USERS', value: '—' },
  { key: 'sessions', label: 'SESSIONS', value: '—' },
  { key: 'roles', label: 'ROLES', value: '—' },
  { key: 'audit', label: 'AUDIT', value: '—' },
]

/** 快捷入口:取菜单前 6 个可路由节点;菜单为空时该卡片自动隐藏。 */
const shortcuts = computed(() => {
  const idx = menu.flatIndex
  return idx.uris.slice(0, 6).map((uri) => ({
    uri,
    title: idx.titlesByUri.get(uri) ?? uri,
  }))
})
</script>

<template>
  <div class="console">
    <!-- 左主区:问候 + 鲸鱼动画 -->
    <section class="glass main">
      <header class="main-head">
        <h1 class="greet">
          {{ greeting }}，<span class="text-gradient">{{ auth.displayName }}</span>
        </h1>
        <p class="date-line">{{ dateLine }}</p>
      </header>

      <div class="stage">
        <svg class="whale-scene" viewBox="0 0 260 150" role="img" aria-label="鲸鱼摆尾动画">
          <defs>
            <clipPath id="whale-body-clip">
              <path d="M30 78 C30 46 68 30 112 32 C154 34 180 52 190 74 C192 78 192 82 190 86 C178 106 148 118 108 116 C64 114 30 102 30 78 Z" />
            </clipPath>
            <linearGradient id="whale-skin" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0%" stop-color="#8ad2b8" />
              <stop offset="100%" stop-color="#57b394" />
            </linearGradient>
          </defs>

          <!-- 波浪:两层异速横移 -->
          <g class="waves" aria-hidden="true">
            <path
              class="wave w1"
              d="M0 126 Q13 120 26 126 T52 126 T78 126 T104 126 T130 126 T156 126 T182 126 T208 126 T234 126 T260 126 T286 126 T312 126 T338 126 T364 126 T390 126 T416 126 T442 126 T468 126 T494 126 T520 126 V150 H0 Z"
            />
            <path
              class="wave w2"
              d="M0 131 Q13 126 26 131 T52 131 T78 131 T104 131 T130 131 T156 131 T182 131 T208 131 T234 131 T260 131 T286 131 T312 131 T338 131 T364 131 T390 131 T416 131 T442 131 T468 131 T494 131 T520 131 V150 H0 Z"
            />
          </g>

          <!-- 鲸鱼本体:整体缓慢浮沉 -->
          <g class="whale">
            <!-- 喷水 -->
            <g class="spout" aria-hidden="true">
              <path class="spout-line" d="M64 34 C64 24 72 22 72 12" />
              <circle class="drop p1" cx="67" cy="14" r="2" />
              <circle class="drop p2" cx="76" cy="11" r="1.6" />
              <circle class="drop p3" cx="71" cy="6" r="1.8" />
            </g>

            <!-- 尾巴:小幅摆动(旋转原点在尾根) -->
            <path
              class="tail"
              d="M190 78
                 C202 62 218 52 234 50
                 C228 62 226 72 230 80
                 C216 78 202 80 190 84
                 C202 88 214 94 222 106
                 C208 104 196 96 188 88
                 C186 84 187 80 190 78 Z"
            />

            <!-- 身体 -->
            <path
              class="body"
              d="M30 78 C30 46 68 30 112 32 C154 34 180 52 190 74 C192 78 192 82 190 86 C178 106 148 118 108 116 C64 114 30 102 30 78 Z"
            />
            <!-- 肚皮:裁剪进身体轮廓 -->
            <ellipse
              class="belly"
              clip-path="url(#whale-body-clip)"
              cx="110" cy="114" rx="82" ry="26"
            />

            <!-- 鳍 -->
            <path class="fin" d="M116 92 C128 87 138 91 141 100 C133 105 121 103 114 96 Z" />

            <!-- 脸:眼睛(会眨)+ 腮红 + 微笑 -->
            <g class="eye-group">
              <circle class="eye" cx="62" cy="70" r="4.2" />
              <circle class="eye-glint" cx="63.5" cy="68.5" r="1.3" />
            </g>
            <circle class="cheek" cx="50" cy="86" r="6.5" />
            <path class="smile" d="M46 93 Q57 101 70 96" />
          </g>
        </svg>
      </div>
    </section>

    <!-- 右侧栏:时钟 / 指标 / 快捷入口 -->
    <aside class="side">
      <section class="glass side-card clock-card">
        <span class="status">
          <i class="status-dot" aria-hidden="true" />
          <span class="micro">SYSTEM ONLINE</span>
        </span>
        <p class="clock">{{ clock }}</p>
      </section>

      <section class="glass side-card">
        <p class="micro">METRICS</p>
        <div class="metric-rows">
          <div v-for="s in stats" :key="s.key" class="metric-row">
            <span class="metric-label">{{ s.label }}</span>
            <span class="metric-dots" aria-hidden="true" />
            <span class="metric-value">{{ s.value }}</span>
          </div>
        </div>
      </section>

      <section v-if="shortcuts.length > 0" class="glass side-card">
        <p class="micro">QUICK ACCESS</p>
        <nav class="quick-list">
          <router-link
            v-for="sc in shortcuts"
            :key="sc.uri"
            :to="sc.uri"
            class="quick-item"
          >
            {{ sc.title }}
          </router-link>
        </nav>
      </section>
    </aside>
  </div>
</template>

<style scoped lang="scss">
.console {
  display: flex;
  gap: 16px;
  width: 100%;
  min-height: 100%;
}

.micro {
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.14em;
  color: var(--ink-2);
}

/* ---- 左主区 ---- */
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 28px;
}

.greet {
  font-family: var(--display);
  font-size: clamp(20px, 2.2vw, 26px);
  font-weight: 700;
  line-height: 1.3;
}

.date-line {
  margin-top: 8px;
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.06em;
  color: var(--ink-2);
}

.stage {
  flex: 1;
  display: grid;
  place-items: center;
  padding-top: 12px;
}

.whale-scene {
  width: min(100%, 440px);
  max-height: 100%;
}

/* ---- 波浪 ---- */
.wave {
  fill: rgba(108, 197, 168, 0.22);
  animation: wave-drift 9s linear infinite;
}

.w2 {
  fill: rgba(108, 197, 168, 0.34);
  animation-duration: 6s;
  animation-direction: reverse;
}

@keyframes wave-drift {
  /* 波形以 26px 为周期铺满两倍画布宽,位移一个画布宽即无缝 */
  to { transform: translateX(-260px); }
}

/* ---- 鲸鱼整体浮沉 ---- */
.whale {
  animation: bob 3.4s ease-in-out infinite alternate;
}

@keyframes bob {
  from { transform: translateY(-2.5px); }
  to { transform: translateY(2.5px); }
}

/* ---- 尾巴小幅摆动:原点钉在尾根(190,80) ---- */
.tail {
  fill: url(#whale-skin);
  transform-box: view-box;
  transform-origin: 190px 80px;
  animation: wag 1.8s ease-in-out infinite alternate;
}

@keyframes wag {
  from { transform: rotate(-5deg); }
  to { transform: rotate(6deg); }
}

.body {
  fill: url(#whale-skin);
}

.belly {
  fill: rgba(255, 255, 255, 0.65);
}

.fin {
  fill: #3f9b7e;
  transform-box: fill-box;
  transform-origin: 20% 20%;
  animation: fin-sway 3.4s ease-in-out infinite alternate;
}

@keyframes fin-sway {
  from { transform: rotate(-4deg); }
  to { transform: rotate(5deg); }
}

/* ---- 喷水:水珠循环上升消散 ---- */
.spout-line {
  fill: none;
  stroke: rgba(108, 197, 168, 0.45);
  stroke-width: 2;
  stroke-linecap: round;
}

.drop {
  fill: var(--acc-mint);
  opacity: 0;
  animation: drop-rise 2.4s ease-out infinite;
}

.p2 { animation-delay: 0.5s; }
.p3 { animation-delay: 1s; }

@keyframes drop-rise {
  0% { opacity: 0; transform: translateY(6px); }
  25% { opacity: 0.9; }
  60% { opacity: 0; transform: translateY(-8px); }
  100% { opacity: 0; }
}

/* ---- 表情 ---- */
.eye-group {
  transform-box: fill-box;
  transform-origin: center;
  animation: blink 4.6s ease-in-out infinite;
}

.eye {
  fill: var(--ink);
}

.eye-glint {
  fill: rgba(255, 255, 255, 0.9);
}

@keyframes blink {
  0%, 93%, 100% { transform: scaleY(1); }
  95.5% { transform: scaleY(0.08); }
  98% { transform: scaleY(1); }
}

.cheek {
  fill: var(--acc-peach);
  opacity: 0.45;
}

.smile {
  fill: none;
  stroke: var(--ink-3);
  stroke-width: 2;
  stroke-linecap: round;
}

/* ---- 右侧栏 ---- */
.side {
  width: 300px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.side-card {
  padding: 20px 24px;
}

.clock-card .clock {
  margin-top: 10px;
  font-family: var(--mono);
  font-size: 34px;
  font-weight: 600;
  color: var(--acc-mint-deep);
  font-variant-numeric: tabular-nums;
  line-height: 1;
}

.status {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--acc-mint);
  box-shadow: 0 0 8px var(--acc-mint);
  animation: pulse 2s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}

/* 指标行:点线引导符 */
.metric-rows {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.metric-row {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.metric-label {
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--ink-2);
}

.metric-dots {
  flex: 1;
  border-bottom: 1px dotted rgba(30, 90, 90, 0.25);
  transform: translateY(-3px);
}

.metric-value {
  font-family: var(--mono);
  font-size: 15px;
  font-weight: 600;
  color: var(--ink);
  font-variant-numeric: tabular-nums;
}

/* 快捷入口 */
.quick-list {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.quick-item {
  padding: 8px 10px;
  margin-left: -10px;
  border-radius: var(--r-sm);
  color: var(--ink-3);
  font-size: 13px;
  text-decoration: none;
  transition: all 0.15s ease;

  &:hover {
    background: var(--menu-active-tint-soft);
    color: var(--acc-mint-deeper);
  }
}

@media (max-width: 900px) {
  .console {
    flex-direction: column;
  }

  .side {
    width: 100%;
  }
}
</style>
