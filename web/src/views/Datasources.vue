<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import Shell from '../components/Shell.vue'
import AdminNav from '../components/AdminNav.vue'
import { api } from '../api'

const router = useRouter()
const items = ref([])
const dialog = ref(false)
const busy = ref(false)
const testResult = ref('')
const form = reactive({ name: '', fileName: '' })

async function load() {
  items.value = await api('/api/datasources')
}

onMounted(load)

function openCreate() {
  form.name = ''
  form.fileName = ''
  testResult.value = ''
  dialog.value = true
}

async function testFile() {
  testResult.value = ''
  try {
    const data = await api('/api/datasources/test', { method: 'POST', body: { fileName: form.fileName.trim() } })
    testResult.value = `可以只读打开，共 ${data.tableCount} 张表`
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function create() {
  busy.value = true
  try {
    const created = await api('/api/datasources', {
      method: 'POST',
      body: { name: form.name.trim(), fileName: form.fileName.trim() },
    })
    dialog.value = false
    router.push(`/admin/datasources/${created.id}`)
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    busy.value = false
  }
}

async function remove(row) {
  try {
    await api(`/api/datasources/${row.id}`, { method: 'DELETE' })
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
        <h1>数据源</h1>
        <el-button type="primary" @click="openCreate">登记</el-button>
      </div>
      <p class="hint">把 SQLite 文件放到 data/sources，这里只填写文件名。服务以只读方式打开它。</p>
      <el-table :data="items" @row-click="(row) => router.push(`/admin/datasources/${row.id}`)">
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="fileName" label="文件名" />
        <el-table-column label="状态">
          <template #default="{ row }">{{ row.status === 1 ? '启用' : '停用' }}</template>
        </el-table-column>
        <el-table-column label="">
          <template #default="{ row }">
            <el-button link type="danger" @click.stop="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-dialog v-model="dialog" title="登记数据源" width="480px">
        <el-form label-position="top">
          <el-form-item label="名称">
            <el-input v-model="form.name" />
          </el-form-item>
          <el-form-item label="文件名">
            <el-input v-model="form.fileName" placeholder="hr.sqlite" />
          </el-form-item>
        </el-form>
        <p v-if="testResult" class="hint">{{ testResult }}</p>
        <template #footer>
          <el-button @click="testFile">测试</el-button>
          <el-button type="primary" :loading="busy" @click="create">登记</el-button>
        </template>
      </el-dialog>
    </div>
  </Shell>
</template>
