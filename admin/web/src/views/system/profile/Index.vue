<script setup lang="ts">
// 与 router/index.ts 里这条静态路由的 meta.componentName (= deriveComponentName
// '@/views/system/profile/Index.vue') 对齐。keep-alive :include=tabs.cachedViews
// 要求稳定可匹配的 name,异步组件包装层一旦丢失 name 就会让缓存失效。
defineOptions({ name: 'SystemProfileIndex' })

import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { Lock, User as UserIcon } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { changePassword as apiChangePassword } from '@/api/user'

const auth = useAuthStore()

// 单数据源:profile 直接绑 auth.userProfile;save/refresh 只写一次 store,
// 任何读端(本页 + header 头像)都看到同一份。
const profile = computed(() => auth.userProfile)

// ── Tab 1:修改资料 ────────────────────────────────────────────
const profileFormRef = ref<FormInstance>()
const profileForm = reactive({
    username: '',
    email: '',
    gender: '',
    description: '',
})
// 留空时后端保留原值,所以"未改字段"用空字符串提交即可,
// 不必再单独搞一份 dirty 标记。
const profileRules: FormRules = {
    username: [
        { required: true, message: '请输入用户名', trigger: 'blur' },
        { min: 2, max: 32, message: '长度在 2 到 32 个字符', trigger: 'blur' },
    ],
    email: [
        { required: true, message: '请输入邮箱', trigger: 'blur' },
        { type: 'email', message: '邮箱格式不正确', trigger: 'blur' },
    ],
}

function fillProfileForm(p: { username?: string; email?: string; gender?: string; description?: string }) {
    Object.assign(profileForm, {
        username: p.username ?? '',
        email: p.email ?? '',
        gender: p.gender ?? '',
        description: p.description ?? '',
    })
}

// 初始填充用当前缓存;之后 store 变化时再同步一次(例如另开 tab 改了资料)。
function syncFromStore() {
    if (auth.userProfile) fillProfileForm(auth.userProfile)
}
onMounted(() => {
    if (!auth.userProfile) void loadProfile()
    else syncFromStore()
})

const loading = ref(false)
async function loadProfile() {
    loading.value = true
    try {
        await auth.fetchProfile()
        syncFromStore()
    } catch {
        // 错误已由 http 拦截器 toast;profile 保留旧值(或 null),避免空态。
    } finally {
        loading.value = false
    }
}

const profileSaving = ref(false)
async function onSaveProfile() {
    if (!profileFormRef.value) return
    const valid = await profileFormRef.value.validate().catch(() => false)
    if (!valid) return
    profileSaving.value = true
    try {
        // 走 store action 写一份,UI 各处(本页 + header)经 auth.userProfile
        // 自动同步;不需要再手动双写一份本地 ref。
        await auth.updateProfile({
            username: profileForm.username.trim(),
            email: profileForm.email.trim(),
            gender: profileForm.gender,
            // description 始终生效(空串即清空),与后端契约一致。
            description: profileForm.description,
        })
        ElMessage.success('资料已更新')
    } catch {
        // 错误由 http 拦截器 toast
    } finally {
        profileSaving.value = false
    }
}

function onResetProfile() {
    syncFromStore()
    profileFormRef.value?.clearValidate()
}

// ── Tab 2:重置密码 ────────────────────────────────────────────
const pwdFormRef = ref<FormInstance>()
const pwdForm = reactive({
    old_password: '',
    new_password: '',
    confirm_password: '',
})
const pwdRules: FormRules = {
    old_password: [{ required: true, message: '请输入当前密码', trigger: 'blur' }],
    new_password: [
        { required: true, message: '请输入新密码', trigger: 'blur' },
        { min: 6, max: 64, message: '长度在 6 到 64 个字符', trigger: 'blur' },
    ],
    confirm_password: [
        { required: true, message: '请再次输入新密码', trigger: 'blur' },
        {
            validator: (_r, value, cb) =>
                cb(value === pwdForm.new_password ? undefined : new Error('两次输入的密码不一致')),
            trigger: 'blur',
        },
    ],
}

const pwdSaving = ref(false)
async function onChangePassword() {
    if (!pwdFormRef.value) return
    const valid = await pwdFormRef.value.validate().catch(() => false)
    if (!valid) return
    pwdSaving.value = true
    try {
        await apiChangePassword({
            old_password: pwdForm.old_password,
            new_password: pwdForm.new_password,
        })
        ElMessage.success('密码已修改,请使用新密码重新登录')
        // 安全考虑:本地缓存 token 不自动失效,提示用户重新登录更稳。
        onResetPassword()
    } catch {
        // 错误由 http 拦截器 toast
    } finally {
        pwdSaving.value = false
    }
}

function onResetPassword() {
    pwdForm.old_password = ''
    pwdForm.new_password = ''
    pwdForm.confirm_password = ''
    pwdFormRef.value?.clearValidate()
}

const activeTab = ref('profile')
</script>

<template>
    <div class="page profile-page">
        <div class="profile-grid">
            <!-- ── 左 1:头像 + 概要 ── -->
            <section class="glass summary">
                <div class="avatar-wrap">
                    <img v-if="profile?.avatar" :src="profile.avatar" :alt="profile.username" class="avatar-img" />
                    <div v-else class="avatar-fallback">{{ auth.initials }}</div>
                </div>

                <div class="summary-name">{{ profile?.username ?? auth.displayName }}</div>
                <div class="summary-role">{{ profile?.role || '—' }}</div>

                <el-button class="reload-btn" size="small" :loading="loading" @click="loadProfile">
                    刷新
                </el-button>

                <el-divider class="summary-divider" />

                <dl class="summary-list">
                    <div class="row">
                        <dt>UID</dt>
                        <dd class="mono">{{ profile?.uid ?? '—' }}</dd>
                    </div>
                    <div class="row">
                        <dt>邮箱</dt>
                        <dd>{{ profile?.email ?? '—' }}</dd>
                    </div>
                    <div class="row">
                        <dt>性别</dt>
                        <dd>{{ profile?.gender || '—' }}</dd>
                    </div>
                    <div class="row">
                        <dt>部门</dt>
                        <dd>{{ profile?.dept_id ?? '—' }}</dd>
                    </div>
                    <div class="row description">
                        <dt>简介</dt>
                        <dd>{{ profile?.description || '这个人很懒,什么都没写。' }}</dd>
                    </div>
                </dl>
            </section>

            <!-- ── 右 3:Tabs ── -->
            <section class="glass tabs-card">
                <el-tabs v-model="activeTab" class="profile-tabs">
                    <el-tab-pane name="profile">
                        <template #label>
                            <span class="tab-label">
                                <el-icon>
                                    <UserIcon />
                                </el-icon>
                                修改资料
                            </span>
                        </template>

                        <el-form ref="profileFormRef" :model="profileForm" :rules="profileRules" label-position="top"
                            class="profile-form" @submit.prevent>
                            <el-form-item label="用户名" prop="username">
                                <el-input v-model="profileForm.username" placeholder="请输入用户名" maxlength="32"
                                    show-word-limit />
                            </el-form-item>
                            <el-form-item label="邮箱" prop="email">
                                <el-input v-model="profileForm.email" placeholder="name@example.com" />
                            </el-form-item>
                            <el-form-item label="性别" prop="gender">
                                <el-radio-group v-model="profileForm.gender">
                                    <el-radio value="M">男</el-radio>
                                    <el-radio value="F">女</el-radio>
                                    <el-radio value="U">未设置</el-radio>
                                </el-radio-group>
                            </el-form-item>
                            <el-form-item label="个人简介" prop="description">
                                <el-input v-model="profileForm.description" type="textarea" :rows="4"
                                    placeholder="一句话介绍一下你自己" maxlength="200" show-word-limit />
                            </el-form-item>
                            <div class="form-actions">
                                <el-button @click="onResetProfile">重置</el-button>
                                <el-button type="primary" :loading="profileSaving" @click="onSaveProfile">
                                    保存修改
                                </el-button>
                            </div>
                        </el-form>
                    </el-tab-pane>

                    <el-tab-pane name="password">
                        <template #label>
                            <span class="tab-label">
                                <el-icon>
                                    <Lock />
                                </el-icon>
                                重置密码
                            </span>
                        </template>

                        <el-form ref="pwdFormRef" :model="pwdForm" :rules="pwdRules" label-position="top"
                            class="profile-form" @submit.prevent>
                            <el-form-item label="当前密码" prop="old_password">
                                <el-input v-model="pwdForm.old_password" type="password" show-password
                                    placeholder="请输入当前密码" autocomplete="current-password" />
                            </el-form-item>
                            <el-form-item label="新密码" prop="new_password">
                                <el-input v-model="pwdForm.new_password" type="password" show-password
                                    placeholder="6 - 64 个字符" autocomplete="new-password" />
                            </el-form-item>
                            <el-form-item label="确认新密码" prop="confirm_password">
                                <el-input v-model="pwdForm.confirm_password" type="password" show-password
                                    placeholder="请再次输入新密码" autocomplete="new-password" />
                            </el-form-item>
                            <div class="form-actions">
                                <el-button @click="onResetPassword">清空</el-button>
                                <el-button type="primary" :loading="pwdSaving" @click="onChangePassword">
                                    修改密码
                                </el-button>
                            </div>
                        </el-form>
                    </el-tab-pane>
                </el-tabs>
            </section>
        </div>
    </div>
</template>

<style scoped>
/* .page 由 app.scss 提供(display:flex; flex-direction:column; gap:16px),
   profile-page 仅追加 max-width + padding。 */
.profile-page {
    max-width: 1080px;
    margin: 0 auto;
    padding: 8px 4px 24px;
    width: 100%;
}

.profile-grid {
    display: grid;
    grid-template-columns: 1fr 3fr;
    gap: 20px;
    align-items: start;
}

@media (max-width: 860px) {
    .profile-grid {
        grid-template-columns: 1fr;
    }
}

/* ── 概要卡(左) ── */
.summary {
    /* .glass 已给背景/边框/阴影/圆角;这里只追加内边距和布局 */
    padding: 28px 22px 22px;
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    border-radius: var(--r-md);
}

.avatar-wrap {
    width: 96px;
    height: 96px;
    border-radius: 50%;
    overflow: hidden;
    display: grid;
    place-items: center;
    background: linear-gradient(135deg, var(--acc-mint), var(--acc-lilac));
    box-shadow: 0 6px 18px var(--shadow-soft);
    margin-bottom: 14px;
}

.avatar-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.avatar-fallback {
    color: white;
    font-weight: 700;
    font-size: 32px;
    letter-spacing: 0.02em;
    font-family: var(--display);
}

.summary-name {
    font-family: var(--display);
    font-size: 18px;
    font-weight: 600;
    color: var(--ink);
    line-height: 1.2;
}

.summary-role {
    margin-top: 4px;
    font-size: 12px;
    color: var(--ink-2);
    letter-spacing: 0.04em;
    text-transform: uppercase;
}

.reload-btn {
    margin-top: 14px;
}

.summary-divider {
    width: calc(100% + 44px);
    margin: 18px -22px;
}

.summary-list {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 10px;
    text-align: left;
}

.summary-list .row {
    display: grid;
    grid-template-columns: 56px 1fr;
    gap: 10px;
    align-items: baseline;
    font-size: 13px;
}

.summary-list .row.description {
    grid-template-columns: 56px 1fr;
    align-items: start;
}

.summary-list dt {
    color: var(--ink-2);
    font-size: 12px;
}

.summary-list dd {
    color: var(--ink);
    word-break: break-word;
}

.summary-list dd.mono {
    font-family: var(--mono);
    font-size: 12px;
}

/* ── Tabs 卡(右) ── */
.tabs-card {
    padding: 6px 22px 22px;
    border-radius: var(--r-md);
}

.profile-tabs {
    --el-tabs-header-height: 48px;
}

.tab-label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
}

.profile-form {
    max-width: 520px;
    margin-top: 8px;
}

.form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    margin-top: 18px;
}

/* el-input__wrapper / el-textarea__inner 的玻璃底色由 styles/glass.scss 全局提供,
   这里不需要再覆盖。 */
</style>
