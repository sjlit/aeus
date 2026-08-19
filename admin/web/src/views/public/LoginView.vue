<script setup lang="ts">
// 与 router/index.ts 里 LoginView 路由 meta.componentName (= deriveComponentName('@/views/public/LoginView.vue')) 对齐。
// 给 <keep-alive :include> 一个稳定可匹配的组件名,防止异步组件包装层丢失 name 导致视图不缓存。
defineOptions({ name: 'PublicLoginView' })

import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
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
  LoginView · 登录页(样式迁移自参考项目 login/index.vue)
  卡片 = .glass 工具类 + 角落两团装饰渐变;品牌行/大标题/脚注均对齐参考。
  渐变文字复用全局 .text-gradient* 工具,不在此重复声明。
-->
<template>
  <div class="login-screen">
    <div class="glass login-card">
      <div class="brand-row">
        <div class="brand-mark">A</div>
        <div class="brand-name text-gradient">aeus</div>
      </div>

      <h1>欢迎回到 <em class="text-gradient--peach">AEUS</em></h1>
      <p class="lead">登录到 AEUS 管理台</p>

      <!-- 提交只走 form submit:输入框回车与按钮点击都由原生 submit 触发 -->
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="uid 或用户名" autofocus size="large" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="至少 6 位" size="large" />
        </el-form-item>

        <div class="form-actions">
          <a href="#">忘记密码？</a>
        </div>

        <el-button type="primary" size="large" native-type="submit" :loading="loading">
          登录
        </el-button>
      </el-form>

      <div class="footer-row">
        <span class="ver">v0.1.0</span>
        <span class="live">系统一切正常</span>
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
  max-width: 440px;
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
  margin-bottom: 32px;
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
  font-size: 32px;
  font-weight: 600;
  letter-spacing: -0.02em;
  margin-bottom: 6px;
  position: relative;
}

.lead {
  font-size: 13px;
  color: var(--ink-2);
  margin-bottom: 28px;
  position: relative;
}

/* label-position="top" 的标签。参考项目是 10px 大写拉丁小标签,
   但这里标签是中文(用户名/密码):mono 无 CJK 字形、10px 过小、
   uppercase 无意义,改为 12px 正文家族。 */
:deep(.el-form-item) {
  margin-bottom: 16px;
}

:deep(.el-form-item__label) {
  display: block;
  height: auto;
  line-height: 1.4;
  padding-bottom: 6px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: var(--ink-2);
}

.form-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 8px 0 24px;
  font-size: 12px;
}

/* 参考项目用 --acc-cobalt(已移除),换成主题主色 */
.form-actions a {
  color: var(--acc-mint);
  font-weight: 500;
  text-decoration: none;
}

.form-actions a:hover {
  color: var(--acc-mint-deep);
}

.el-button {
  width: 100%;
  height: 48px;
  font-size: 14px;
  letter-spacing: 0.05em;
}

.footer-row {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  letter-spacing: 0.04em;
  color: var(--ink-2);
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px dashed rgba(30, 90, 90, 0.15);
  position: relative;
}

/* 版本号是纯拉丁,保留 mono 味道;状态文案是中文,随正文家族 */
.footer-row .ver {
  font-family: var(--mono);
}

.live {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--acc-mint);
}

.live::before {
  content: '';
  width: 6px;
  height: 6px;
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
