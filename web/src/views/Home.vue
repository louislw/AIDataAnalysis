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
    <div class="page">
      <h1>问答</h1>
      <p v-if="empty">还没有可用的人物。</p>
      <router-link v-if="empty && session.isAdmin" to="/admin/personas">去配置人物</router-link>
    </div>
  </Shell>
</template>
