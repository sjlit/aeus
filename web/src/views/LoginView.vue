<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { useAuthStore } from '../stores/auth'

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

<template>
  <div class="login-wrap">
    <div class="login-card">
      <div class="login-brand text-gradient">aeus</div>
      <p class="login-sub">AEUS Admin</p>
      <!-- 提交只走 form submit:输入框回车与按钮点击都由原生 submit 触发 -->
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="uid 或用户名" autofocus size="large" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="至少 6 位" size="large" />
        </el-form-item>
        <el-button type="primary" size="large" class="login-btn" native-type="submit" :loading="loading">
          登录
        </el-button>
      </el-form>
    </div>
  </div>
</template>

<style scoped>
.login-wrap {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 24px;
}
.login-card {
  width: 100%;
  max-width: 420px;
  background: var(--glass-strong);
  backdrop-filter: blur(30px) saturate(150%);
  border: 1px solid var(--glass-border);
  border-radius: 24px;
  box-shadow: 0 16px 48px var(--shadow);
  padding: 40px 36px;
}
.login-brand {
  font-family: var(--display);
  font-size: 28px;
  font-weight: 600;
  letter-spacing: -0.02em;
}
.login-sub {
  font-size: 12px;
  color: var(--ink-2);
  letter-spacing: 0.15em;
  text-transform: uppercase;
  margin: 4px 0 28px;
}
.login-btn { width: 100%; margin-top: 8px; }
</style>
