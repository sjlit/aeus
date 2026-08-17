import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import { router } from './router'
import { bindHttpContext } from './api/http'
import { useAuthStore } from './stores/auth'
import './styles/app.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)
app.use(ElementPlus)

const auth = useAuthStore()
bindHttpContext({
  getToken: () => auth.accessToken,
  getTenantId: () => auth.profile?.tenant_id ?? null,
  refresh: () => auth.refresh(),
  logout: () => void auth.logout(),
  pushLogin: (redirect) => {
    void router.push({ path: '/login', query: { redirect } })
  },
})

app.mount('#app')