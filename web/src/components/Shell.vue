<script setup>
import { useRouter } from 'vue-router'
import { useSession } from '../stores/session'

const session = useSession()
const router = useRouter()

function logout() {
  session.clear()
  router.push('/login')
}
</script>

<template>
  <div class="shell">
    <aside class="side">
      <div class="brand">数据分析</div>
      <div class="side-scroll">
        <slot name="side" />
      </div>
      <div class="side-foot">
        <span>{{ session.displayName || session.username }}</span>
        <router-link v-if="session.isAdmin" to="/admin/datasources">管理</router-link>
        <router-link to="/">问答</router-link>
        <button class="linkish" type="button" @click="logout">退出</button>
      </div>
    </aside>
    <main class="main">
      <slot />
    </main>
  </div>
</template>
