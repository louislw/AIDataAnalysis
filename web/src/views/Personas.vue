<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import Shell from '../components/Shell.vue'
import AdminNav from '../components/AdminNav.vue'
import { api } from '../api'

const router = useRouter()
const items = ref([])
const sources = ref([])
const dialog = ref(false)
const busy = ref(false)
const form = reactive({ name: '', datasourceId: null })

async function load() {
  items.value = await api('/api/personas')
  sources.value = await api('/api/datasources')
}

onMounted(async () => {
  try {
    await load()
  } catch (err) {
    ElMessage.error(err.message)
  }
})

function sourceName(id) {
  return sources.value.find((item) => item.id === id)?.name || ''
}

function openCreate() {
  form.name = ''
  form.datasourceId = null
  dialog.value = true
}

async function create() {
  busy.value = true
  try {
    const created = await api('/api/personas', {
      method: 'POST',
      body: {
        name: form.name.trim(),
        description: '',
        systemPrompt: '',
        prohibitions: '',
        rowFilter: '',
        datasourceId: form.datasourceId || 0,
        status: 1,
      },
    })
    dialog.value = false
    router.push(`/admin/personas/${created.id}`)
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    busy.value = false
  }
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(`删除「${row.name}」后不可恢复。`, '删除人物', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonType: 'danger',
    })
  } catch {
    return
  }
  try {
    await api(`/api/personas/${row.id}`, { method: 'DELETE' })
    await load()
  } catch (err) {
    ElMessage.error(err.message)
  }
}
</script>

<template>
  <Shell>
    <template #side>
      <AdminNav />
    </template>
    <div class="page">
      <div class="page-head">
        <div>
          <p class="kicker">管理</p>
          <h1>人物</h1>
          <p class="lede">每个人物绑定一个数据源，并决定能看哪些表、怎么解释口径、行级条件是什么。</p>
        </div>
        <el-button type="primary" @click="openCreate">新建</el-button>
      </div>
      <div v-if="!items.length" class="empty-state">
        <h2>还没有人物</h2>
        <p>先登记数据源，再创建人物并授权表。</p>
      </div>
      <div v-else class="card-grid">
        <article v-for="row in items" :key="row.id" class="entity-card">
          <router-link class="entity-link" :to="`/admin/personas/${row.id}`">
            <div class="entity-top">
              <h2>{{ row.name }}</h2>
              <span class="pill" :class="row.status === 1 ? 'ok' : 'off'">{{ row.status === 1 ? '启用' : '停用' }}</span>
            </div>
            <p class="meta">{{ sourceName(row.datasourceId) || '未绑定数据源' }}</p>
          </router-link>
          <el-button class="text-danger" link type="danger" @click="remove(row)">删除</el-button>
        </article>
      </div>
      <el-dialog v-model="dialog" title="新建人物" width="440px">
        <el-form label-position="top">
          <el-form-item label="名称">
            <el-input v-model="form.name" />
          </el-form-item>
          <el-form-item label="数据源">
            <el-select v-model="form.datasourceId" clearable placeholder="可稍后选择" style="width: 100%">
              <el-option v-for="item in sources" :key="item.id" :label="item.name" :value="item.id" />
            </el-select>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button type="primary" :loading="busy" @click="create">创建</el-button>
        </template>
      </el-dialog>
    </div>
  </Shell>
</template>
