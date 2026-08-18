<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'

const route = useRoute()
const title = computed(() => (route.meta.title as string) ?? route.path)
</script>

<template>
  <section class="glass panel">
    <h3>{{ title }} <span class="tag">PLACEHOLDER</span></h3>
    <p class="desc">
      此页面由 MenuTree 驱动：菜单 uri <code>{{ route.path }}</code> 即客户端路由。
      后续在此挂载对应 REST 资源页面（用户 / 角色 / 权限 / 租户等）。
    </p>
    <el-button type="primary" @click="ElMessage.info(`即将接入 ${route.path} 的资源页面`)">
      查看接口文档
    </el-button>
  </section>
</template>

<style scoped>
.panel h3 {
  font-family: var(--display);
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.panel .tag {
  font-family: var(--body);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.15em;
  color: var(--ink-2);
  text-transform: uppercase;
}
.panel .desc {
  font-size: 13px;
  color: var(--ink-2);
  margin-bottom: 16px;
  line-height: 1.6;
}
.panel code {
  font-family: var(--mono);
  font-size: 12px;
  background: rgba(0, 0, 0, 0.05);
  padding: 2px 6px;
  border-radius: 6px;
}
</style>
