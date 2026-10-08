<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const active = computed(() => {
  if (route.path.startsWith('/admin/personas')) return '/admin/personas'
  if (route.path.startsWith('/admin/datasources')) return '/admin/datasources'
  return '/'
})
</script>

<template>
  <el-menu
    :default-active="active"
    router
    background-color="#1f2a37"
    text-color="#e5eaf3"
    active-text-color="#ffffff"
  >
    <el-menu-item index="/admin/datasources">数据源</el-menu-item>
    <el-menu-item index="/admin/personas">人物</el-menu-item>
    <el-menu-item index="/">返回问答</el-menu-item>
  </el-menu>
</template>
