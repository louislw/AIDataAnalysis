<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
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
    await ElMessageBox.confirm(`删除「${row.name}」后不可恢复。`, '删除数据源', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonType: 'danger',
    })
  } catch {
    return
  }
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
        <div>
          <p class="kicker">管理</p>
          <h1>数据源</h1>
          <p class="lede">把 SQLite 文件放到 data/sources，这里只填写文件名。服务以只读方式打开它。</p>
        </div>
        <el-button type="primary" @click="openCreate">登记</el-button>
      </div>
      <div v-if="!items.length" class="empty-state">
        <h2>还没有数据源</h2>
        <p>登记文件名之后，可以为表和字段补注释。</p>
      </div>
      <div v-else class="card-grid">
        <article v-for="row in items" :key="row.id" class="entity-card">
          <router-link class="entity-link" :to="`/admin/datasources/${row.id}`">
            <div class="entity-top">
              <h2>{{ row.name }}</h2>
              <span class="pill" :class="row.status === 1 ? 'ok' : 'off'">{{ row.status === 1 ? '启用' : '停用' }}</span>
            </div>
            <p class="meta mono">{{ row.fileName }}</p>
          </router-link>
          <el-button class="text-danger" link type="danger" @click="remove(row)">删除</el-button>
        </article>
      </div>
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
