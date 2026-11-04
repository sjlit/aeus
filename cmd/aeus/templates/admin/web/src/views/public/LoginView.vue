<script setup lang="ts">
// 与 router/index.ts 里 LoginView 路由 meta.componentName (= deriveComponentName('@/views/public/LoginView.vue')) 对齐。
// 给 <keep-alive :include> 一个稳定可匹配的组件名,防止异步组件包装层丢失 name 导致视图不缓存。
defineOptions({ name: 'PublicLoginView' })

import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { Lock, User } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({ username: 'admin', password: '' })

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '密码至少 6 位', trigger: 'blur' },
  ],
}

async function submit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    // 换用户后菜单可能不同:auth.login 已清空菜单缓存,路由守卫会在
    // 这次导航里重新拉取;落地页由守卫的 '/' 分支统一决定。
    await auth.login(form)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect)
  } catch {
    // 业务错误已由 http 拦截器 toast
  } finally {
    loading.value = false
  }
}
</script>

<!--
  LoginView · 登录页
  卡片 = .glass 工具类 + 角落两团装饰渐变;文案做减法:
  无副标题、无表单 label(图标前缀 + placeholder 承担语义)、无找回入口。
  渐变文字复用全局 .text-gradient* 工具,不在此重复声明。
-->
<template>
  <div class="login-screen">
    <div class="glass login-card">
      <div class="brand-row">
        <div class="brand-mark">A</div>
        <div class="brand-name text-gradient">aeus</div>
      </div>

      <h1>欢迎回来</h1>

      <!-- 提交只走 form submit:输入框回车与按钮点击都由原生 submit 触发 -->
      <el-form ref="formRef" :model="form" :rules="rules" @submit.prevent="submit">
        <el-form-item prop="username">
          <el-input v-model="form.username" placeholder="用户名" :prefix-icon="User" autofocus size="large" />
        </el-form-item>
        <el-form-item prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="密码" :prefix-icon="Lock" size="large" />
        </el-form-item>

        <el-button type="primary" size="large" native-type="submit" :loading="loading">
          登 录
        </el-button>
      </el-form>

      <div class="footer-row">
        <span class="live" title="系统一切正常" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-screen {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 28px;
}

.login-card {
  width: 100%;
  max-width: 400px;
  padding: 44px 40px;
  position: relative;
  overflow: hidden;
}

/* 角落两团装饰渐变:蜜桃右上 + 丁香左下,blur 后透出毛玻璃(参考项目同款) */
.login-card::before {
  content: '';
  position: absolute;
  top: -80px;
  right: -80px;
  width: 240px;
  height: 240px;
  background: radial-gradient(circle, var(--acc-peach), transparent 70%);
  filter: blur(40px);
  opacity: 0.5;
  pointer-events: none;
}

.login-card::after {
  content: '';
  position: absolute;
  bottom: -60px;
  left: -60px;
  width: 200px;
  height: 200px;
  background: radial-gradient(circle, var(--acc-lilac), transparent 70%);
  filter: blur(40px);
  opacity: 0.4;
  pointer-events: none;
}

.brand-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 28px;
  position: relative;
}

.brand-mark {
  width: 28px;
  height: 28px;
  background: linear-gradient(135deg, var(--acc-mint), var(--acc-lilac));
  border-radius: 8px;
  display: grid;
  place-items: center;
  color: white;
  font-weight: 700;
  font-size: 14px;
  box-shadow: 0 4px 12px rgba(108, 197, 168, 0.4);
}

.brand-name {
  font-family: var(--display);
  font-size: 22px;
  font-weight: 600;
  letter-spacing: -0.02em;
}

h1 {
  font-family: var(--display);
  font-size: 30px;
  font-weight: 600;
  letter-spacing: -0.02em;
  margin-bottom: 24px;
  position: relative;
}

/* 表单抬到装饰光斑之上:卡片 ::before/::after 是定位元素,
   未定位的 in-flow 内容会被其罩色(登录按钮发灰),补一层定位。 */
.el-form {
  position: relative;
}

:deep(.el-form-item) {
  margin-bottom: 16px;
}

/* 输入框去边框感、加柔和底色,与玻璃卡片更融合 */
:deep(.el-input__wrapper) {
  border-radius: var(--r-md);
  padding: 4px 14px;
  box-shadow: 0 0 0 1px var(--glass-border) inset;
  background: rgba(255, 255, 255, 0.55);
  transition: box-shadow 0.2s ease, background 0.2s ease;
}

:deep(.el-input__wrapper:hover) {
  box-shadow: 0 0 0 1px var(--acc-mint) inset;
}

:deep(.el-input__wrapper.is-focus) {
  background: rgba(255, 255, 255, 0.85);
  box-shadow: var(--ring), 0 0 0 1px var(--acc-mint) inset;
}

:deep(.el-input__prefix .el-icon) {
  color: var(--ink-2);
}

.el-button {
  width: 100%;
  height: 48px;
  margin-top: 8px;
  font-size: 14px;
  letter-spacing: 0.35em;
  text-indent: 0.35em;
  border-radius: var(--r-md);
}

.footer-row {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  font-size: 11px;
  letter-spacing: 0.04em;
  color: var(--ink-2);
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px dashed rgba(30, 90, 90, 0.15);
  position: relative;
}

.live {
  width: 8px;
  height: 8px;
  background: var(--acc-mint);
  border-radius: 50%;
  box-shadow: 0 0 8px var(--acc-mint);
  animation: pulse 1.6s infinite;
}

@keyframes pulse {

  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }

  50% {
    opacity: 0.4;
    transform: scale(1.4);
  }
}
</style>
