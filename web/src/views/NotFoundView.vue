<script setup lang="ts">
import { useRouter } from 'vue-router'
const router = useRouter()
</script>

<template>
  <section class="glass panel notfound">
    <h1 class="text-gradient--peach">404</h1>
    <p>页面不存在。菜单外的地址不会出现在这里——业务路由全部来自 MenuTree。</p>
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
