<script setup>
import { useRoute } from 'vue-router'

const route = useRoute()
</script>

<template>
  <div class="side-label">管理</div>
  <router-link
    to="/admin/datasources"
    class="nav-btn"
    :class="{ on: route.path.startsWith('/admin/datasources') }"
  >数据源</router-link>
  <router-link
    to="/admin/personas"
    class="nav-btn"
    :class="{ on: route.path.startsWith('/admin/personas') }"
  >人物</router-link>
  <router-link to="/" class="nav-btn">返回问答</router-link>
</template>
