<script setup lang="ts">
// EP 走 unplugin-vue-components 按需引入、没有 app.use(ElementPlus),
// 全局配置(语言等)只能用 ElConfigProvider 包裹根节点下发。
import zhCn from 'element-plus/es/locale/lang/zh-cn'
</script>
<template>
  <el-config-provider :locale="zhCn">
    <router-view />
  </el-config-provider>
</template>
