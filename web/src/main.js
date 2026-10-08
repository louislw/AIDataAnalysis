import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
import '@fontsource/noto-sans-sc/latin-400.css'
import '@fontsource/noto-sans-sc/latin-500.css'
import '@fontsource/noto-sans-sc/latin-700.css'
import '@fontsource/noto-sans-sc/chinese-simplified-400.css'
import '@fontsource/noto-sans-sc/chinese-simplified-500.css'
import '@fontsource/noto-sans-sc/chinese-simplified-700.css'
import '@fontsource/noto-serif-sc/latin-600.css'
import '@fontsource/noto-serif-sc/chinese-simplified-600.css'
import App from './App.vue'
import router from './router'
import { useSession } from './stores/session'
import { api } from './api'
import './styles.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)
app.use(ElementPlus, { locale: zhCn })

const session = useSession()
api('/api/health')
  .then((health) => session.setTimeoutSeconds(health.llmTimeoutSeconds))
  .catch(() => {})

app.mount('#app')
