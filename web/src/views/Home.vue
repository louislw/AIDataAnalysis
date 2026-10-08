<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { useSession } from '../stores/session'
import Shell from '../components/Shell.vue'

const router = useRouter()
const session = useSession()
const empty = ref(false)

onMounted(async () => {
  try {
    const list = await api('/api/personas/mine')
    const last = localStorage.getItem('ada.lastPersona')
    const hit = (list || []).find((item) => String(item.id) === last) || (list || [])[0]
    if (hit) {
      router.replace(`/p/${hit.id}`)
      return
    }
    empty.value = true
  } catch (err) {
    empty.value = true
  }
})
</script>

<template>
  <Shell>
    <template #side>
      <div class="side-label">人物</div>
      <p class="side-empty">配置完成后会出现在这里。</p>
    </template>
    <div class="page center-page">
      <p v-if="!empty" class="hint">正在打开最近的人物…</p>
      <div v-else class="empty-state">
        <h1>还没有可用的人物。</h1>
        <p>管理员配置人物并完成授权后，可以从这里开始提问。</p>
        <router-link v-if="empty && session.isAdmin" class="btn-link" to="/admin/personas">去配置人物</router-link>
      </div>
    </div>
  </Shell>
</template>
