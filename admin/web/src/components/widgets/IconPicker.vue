<!--
  IconPicker.vue — el-select 风格的 element-plus 图标选择器。

  用法:
    <IconPicker v-model="form.icon" placeholder="选择图标" clearable />

  行为契约(刻意对齐 el-select):
    - v-model:string     双向绑定当前图标名(PascalCase,与 element-plus
                         一致;后端 menu.icon 兼容 raw 短串,见 utils/icons.ts)
    - placeholder:string 占位文本
    - clearable:boolean  是否显示清除按钮(默认 true)
    - disabled:boolean   禁用
    - width:string|number  触发器宽度,默认 '100%'

  事件:
    - update:modelValue  标准 v-model 协议
    - change             与 el-select 一致(选择后触发;清除也会触发)
    - clear              清除时单独触发(便于埋点/校验)

  面板生命周期:
    - 打开:点触发器(el-popover trigger="click" 自己处理,不要在触发器
      上挂额外 @click——会和 el-popover 内部的 click 监听都翻 visible,
      净效果等于没翻,导致面板"闪一下就消失")
    - 关闭:点面板外(自带)、显式"完成"按钮
    - 选图标后**不自动关闭**——浏览器/选择型 picker 习惯,允许连续切换
      比较后再决定;确认靠完成按钮或点外面

  设计取舍:
    - 用 el-popover trigger="click" 提供点外自动关闭;面板 portal 到 body,
      不会挤压触发器所在布局。
    - 图标全集来自 * as ElIcons;Vite 会按需分块,不在引用的会进单独 chunk,
      体积可接受。改图标库只需换 import 源。
    - 不做虚拟滚动:全集 294 个,过滤后通常 <50 个 DOM 节点;若以后接入
      大量自定义图标库再考虑。
-->
<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { Component } from 'vue'
import { ElButton, ElInput, ElPopover } from 'element-plus'
import * as ElIcons from '@element-plus/icons-vue'
import { toPascal } from '../../utils/icons'

defineOptions({ name: 'IconPicker' })

const props = withDefaults(
  defineProps<{
    modelValue?: string
    placeholder?: string
    clearable?: boolean
    disabled?: boolean
    width?: string | number
  }>(),
  {
    placeholder: '请选择图标',
    clearable: true,
    disabled: false,
    width: '210px',
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  change: [value: string]
  clear: []
}>()

// ALL: PascalCase 图标全集。kebab-case 文件名会规整为 PascalCase 后,
// 运行时也是 PascalCase 注册。这里再做一遍"首字母大写"过滤,避开
// icons-vue 在不同版本里偶尔混入的小写别名(没有也安全)。每条名
// 字对应的 lowercased 版本预计算一次,过滤时只走 includes,省掉
// 每次按键都重做 toLowerCase 的 O(N·k) 工作。
const ICONS = ElIcons as unknown as Record<string, Component>
const ALL_NAMES = Object.keys(ICONS)
  .filter((k) => /^[A-Z][A-Za-z0-9]*$/.test(k))
  .sort()
const ALL_NAMES_LC: string[] = ALL_NAMES.map((n) => n.toLowerCase())

// 关闭 / 搜索 / 箭头三个静态图标直接取自同一份 namespace,避免再多写一份具名 import;
// 同时跟下面网格里的图标保持单一来源——换图标库时只改 import 即可。
const CloseIcon = ICONS.Close
const SearchIcon = ICONS.Search
const ArrowIcon = ICONS.ArrowDown

/** 当前 v-model 命中的 PascalCase;若解析不到(用户手动敲了不存在的串)
 *  返回空串,触发器渲染"未匹配"占位,而不是直接抛错。 */
const resolved = computed<string>(() => {
  const v = props.modelValue
  if (!v) return ''
  if (ALL_NAMES.includes(v)) return v
  const p = toPascal(v)
  return ALL_NAMES.includes(p) ? p : ''
})

const selectedComponent = computed<Component | null>(() => {
  const name = resolved.value
  return name ? ICONS[name] : null
})

// 面板状态 + 搜索。visible 由 el-popover v-model:visible 双向绑定;
// 不在触发器上挂 @click——el-popover 的 trigger="click" 已经处理。
const visible = ref(false)
const query = ref('')
const searchRef = ref<InstanceType<typeof ElInput> | null>(null)

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return ALL_NAMES
  const out: string[] = []
  for (let i = 0; i < ALL_NAMES.length; i++) {
    if (ALL_NAMES_LC[i]!.includes(q)) out.push(ALL_NAMES[i]!)
  }
  return out
})

watch(visible, async (open) => {
  if (open) {
    query.value = ''
    await nextTick()
    searchRef.value?.focus()
  }
})

/** 选择图标——保持面板打开,允许用户连续点击浏览;重复点同一项 no-op。 */
function pick(name: string): void {
  if (name === resolved.value) return
  emit('update:modelValue', name)
  emit('change', name)
}

/** 触发器右侧的 × — 清除选择,面板保持打开让用户继续挑。 */
function clear(): void {
  if (!resolved.value) return
  emit('update:modelValue', '')
  emit('change', '')
  emit('clear')
}

/** 面板底部"完成"按钮:显式关闭。点外面会自带关闭,这里是冗余但显式。 */
function done(): void {
  visible.value = false
}
</script>

<template>
  <el-popover v-model:visible="visible" placement="bottom-start" :width="380" :show-arrow="false" trigger="click"
    :disabled="disabled" popper-class="ip-popover">
    <template #reference>
      <!-- 触发器:el-popover trigger="click" 会自己监听 reference 的 click,
           这里**不要**再挂 @click——会和 el-popover 内部的 listener 各翻
           一次 visible,净效果等于没翻,面板"闪一下就消失"。
           不要借用 el-select__wrapper 类名:glass.scss 对它有 !important
           玻璃覆盖,会压掉下面自绘的 hover/focus/disabled 描边。 -->
      <div class="ip-trigger" :class="{ 'is-disabled': disabled, 'is-focused': visible }"
        :style="{ width }">
        <span class="ip-trigger__prefix">
          <el-icon v-if="selectedComponent" class="ip-icon">
            <component :is="selectedComponent" />
          </el-icon>
        </span>
        <span class="ip-trigger__selection">
          <span v-if="resolved" class="ip-trigger__name">{{ resolved }}</span>
          <span v-else class="ip-trigger__placeholder">{{ placeholder }}</span>
        </span>
        <span class="ip-trigger__suffix">
          <button v-if="clearable && resolved && !disabled" type="button"
            class="ip-trigger__clear el-select__caret is-clear" aria-label="清除" @click.stop="clear">
            <el-icon>
              <component :is="CloseIcon" />
            </el-icon>
          </button>
          <span v-if="!(clearable && resolved && !disabled)" class="ip-trigger__caret el-select__caret"
            :class="{ 'is-reverse': visible }">
            <el-icon>
              <component :is="ArrowIcon" />
            </el-icon>
          </span>
        </span>
      </div>
    </template>

    <div class="ip-panel">
      <ElInput ref="searchRef" v-model="query" size="default" placeholder="搜索图标名称" clearable class="ip-panel__search">
        <template #prefix>
          <el-icon>
            <component :is="SearchIcon" />
          </el-icon>
        </template>
      </ElInput>

      <div v-if="filtered.length === 0" class="ip-panel__empty">无匹配图标</div>
      <div v-else class="ip-panel__grid">
        <button v-for="name in filtered" :key="name" type="button" class="ip-cell"
          :class="{ 'is-selected': name === resolved }" :title="name" @click="pick(name)">
          <el-icon class="ip-icon">
            <component :is="ICONS[name]" />
          </el-icon>
          <span class="ip-cell__label">{{ name }}</span>
        </button>
      </div>

      <footer class="ip-panel__footer">
        <span class="ip-panel__hint">
          <template v-if="resolved">已选:<strong>{{ resolved }}</strong></template>
          <template v-else>点击图标即可选择</template>
        </span>
        <span class="ip-panel__actions">
          <ElButton v-if="clearable && resolved" size="small" link @click="clear">清除</ElButton>
          <ElButton size="small" type="primary" @click="done">完成</ElButton>
        </span>
      </footer>
    </div>
  </el-popover>
</template>

<style scoped>
/* ------------------------------------------------------------------
 * 触发器 —— 复刻 .el-select__wrapper
 *   - 边框用 inset box-shadow 而不是 border(对齐 element-plus 的
 *     mixed-input-border mixin)
 *   - translateZ(0) 抑制 Chromium inset 阴影噪点
 *   - padding / min-height / gap 全部取自 $select-wrapper-padding /
 *     $input-height / $select-item-gap default
 * ------------------------------------------------------------------ */
.ip-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 12px;
  min-height: 32px;
  border-radius: var(--el-border-radius-base);
  background-color: var(--el-fill-color-blank);
  box-shadow: 0 0 0 1px var(--el-border-color) inset;
  cursor: pointer;
  font-size: var(--el-font-size-base);
  line-height: 24px;
  transform: translateZ(0);
  transition: box-shadow var(--el-transition-duration);
  user-select: none;
}

.ip-trigger:hover:not(.is-disabled):not(.is-focused) {
  box-shadow: 0 0 0 1px var(--el-border-color-hover) inset;
}

.ip-trigger.is-focused {
  box-shadow: 0 0 0 1px var(--el-color-primary) inset;
}

.ip-trigger.is-disabled {
  background-color: var(--el-fill-color-light);
  color: var(--el-text-color-placeholder);
  box-shadow: 0 0 0 1px var(--el-disabled-border-color) inset;
  cursor: not-allowed;
}

.ip-trigger__prefix {
  display: inline-flex;
  align-items: center;
  flex-shrink: 0;
  color: var(--el-text-color-regular);
}

.ip-trigger__selection {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
}

.ip-trigger__name {
  flex: 1;
  min-width: 0;
  color: var(--el-text-color-regular);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ip-trigger__placeholder {
  display: block;
  width: 100%;
  color: var(--el-text-color-placeholder);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ip-trigger__suffix {
  display: inline-flex;
  align-items: center;
  flex-shrink: 0;
  gap: 4px;
  color: var(--el-input-icon-color, var(--el-text-color-placeholder));
}

.ip-trigger__caret {
  display: inline-flex;
  align-items: center;
  transition: transform var(--el-transition-duration);
}

.ip-trigger__caret.is-reverse {
  transform: rotateZ(180deg);
}

.ip-trigger__clear {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  padding: 0;
  font-size: inherit;
  color: inherit;
  cursor: pointer;
}

.ip-trigger__clear:hover {
  color: var(--el-select-close-hover-color, var(--el-text-color-secondary));
}

.ip-icon {
  font-size: 18px;
}
</style>

<!--
  非 scoped 样式:面板由 el-popover teleport 到 body 末尾,scoped 属性不会
  作用到那里。命名空间以 ip- 前缀避免污染全局。
  下拉容器复刻 .el-select-dropdown:overlay bg + box-shadow-light + base 圆角,
  自身 padding 为 0,内边距交给 .ip-panel 控。
-->
<style>
.ip-popover {
  padding: 0 !important;
  background-color: var(--el-bg-color-overlay) !important;
  border-radius: var(--el-border-radius-base);
  box-shadow: var(--el-box-shadow-light);
}

.ip-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px;
}

.ip-panel__search {
  width: 100%;
}

.ip-panel__empty {
  padding: 24px 0;
  text-align: center;
  color: var(--el-text-color-secondary);
  font-size: var(--el-font-size-base);
}

.ip-panel__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(72px, 1fr));
  gap: 2px;
  max-height: 274px;
  /* 与 $select-dropdown max-height 一致 */
  overflow-y: auto;
  padding: 2px 0;
}

.ip-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 8px 4px;
  border: none;
  background: transparent;
  border-radius: 0;
  cursor: pointer;
  color: var(--el-text-color-regular);
  font-size: var(--el-font-size-base);
  transition: background-color var(--el-transition-duration), color var(--el-transition-duration);
}

.ip-cell:hover {
  background-color: var(--el-fill-color-light);
}

.ip-cell.is-selected {
  color: var(--el-color-primary);
  font-weight: 500;
  background-color: var(--el-fill-color-light);
}

.ip-cell__label {
  font-size: 11px;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ip-panel__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--el-border-color-lighter);
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.ip-panel__hint strong {
  color: var(--el-color-primary);
  font-weight: 500;
}

.ip-panel__actions {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
</style>