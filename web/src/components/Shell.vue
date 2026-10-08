<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSession } from '../stores/session'
import Mark from './Mark.vue'

const session = useSession()
const router = useRouter()
const route = useRoute()
const media = window.matchMedia('(max-width: 900px)')
const narrow = ref(media.matches)
const open = ref(false)

const onAdmin = computed(() => route.path.startsWith('/admin'))
const initial = computed(() => {
  const name = session.displayName || session.username || '?'
  return Array.from(name)[0] || '?'
})

function onChange() {
  narrow.value = media.matches
  if (!media.matches) open.value = false
}

onMounted(() => media.addEventListener('change', onChange))
onUnmounted(() => media.removeEventListener('change', onChange))
watch(() => route.fullPath, () => {
  open.value = false
})

function logout() {
  session.clear()
  router.push('/login')
}
</script>

<template>
  <div class="shell" :class="{ 'nav-open': open }">
    <header v-if="narrow" class="mobile-bar">
      <button type="button" class="nav-toggle" :aria-expanded="open" @click="open = true">菜单</button>
      <span class="mobile-title">数据分析</span>
    </header>
    <div v-if="narrow && open" class="scrim" @click="open = false"></div>
    <aside class="side">
      <div class="brand-row">
        <router-link to="/" class="brand">
          <Mark />
          <span>数据分析</span>
        </router-link>
        <button v-if="narrow" type="button" class="nav-close" @click="open = false">关闭</button>
      </div>
      <div class="side-scroll">
        <slot name="side" />
      </div>
      <div class="side-foot">
        <div class="whoami">
          <span class="avatar">{{ initial }}</span>
          <div class="who-meta">
            <strong>{{ session.displayName || session.username }}</strong>
            <small>{{ session.isAdmin ? '管理员' : '用户' }}</small>
          </div>
        </div>
        <div class="foot-actions">
          <router-link to="/" class="foot-link" :class="{ on: !onAdmin }">问答</router-link>
          <router-link v-if="session.isAdmin" to="/admin/datasources" class="foot-link" :class="{ on: onAdmin }">管理</router-link>
          <button class="foot-link linkish" type="button" @click="logout">退出</button>
        </div>
      </div>
    </aside>
    <main class="main">
      <slot />
    </main>
  </div>
</template>
