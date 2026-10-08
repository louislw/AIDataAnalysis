<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
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
        <h1>人物</h1>
        <el-button type="primary" @click="dialog = true">新建</el-button>
      </div>
      <el-table :data="items" @row-click="(row) => router.push(`/admin/personas/${row.id}`)">
        <el-table-column prop="name" label="名称" />
        <el-table-column label="数据源">
          <template #default="{ row }">{{ sourceName(row.datasourceId) }}</template>
        </el-table-column>
        <el-table-column label="状态">
          <template #default="{ row }">{{ row.status === 1 ? '启用' : '停用' }}</template>
        </el-table-column>
        <el-table-column width="90">
          <template #default="{ row }">
            <el-button link type="danger" @click.stop="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-dialog v-model="dialog" title="新建人物" width="420px">
        <el-form label-position="top">
          <el-form-item label="名称">
            <el-input v-model="form.name" />
          </el-form-item>
          <el-form-item label="数据源">
            <el-select v-model="form.datasourceId" clearable placeholder="可稍后选择" style="width:100%">
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
