<script setup>
import { onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import Shell from '../components/Shell.vue'
import AdminNav from '../components/AdminNav.vue'
import { api } from '../api'

const route = useRoute()
const tables = ref([])
const selected = ref('')
const tableForm = ref({ comment: '', description: '', hidden: false })
const columns = ref([])
const preview = ref({ columns: [], rows: [] })
const saving = ref(false)

async function loadTables() {
  const data = await api(`/api/datasources/${route.params.id}/tables`)
  tables.value = data.tables || []
  if (!selected.value && tables.value.length) selectTable(tables.value[0].name)
}

async function selectTable(name) {
  selected.value = name
  const table = tables.value.find((item) => item.name === name)
  tableForm.value = {
    comment: table?.comment || '',
    description: table?.description || '',
    hidden: !!table?.hidden,
  }
  const cols = await api(`/api/datasources/${route.params.id}/tables/${encodeURIComponent(name)}/columns`)
  columns.value = cols.columns || []
  preview.value = await api(`/api/datasources/${route.params.id}/tables/${encodeURIComponent(name)}/preview`)
}

async function rebuildCatalogs() {
  const personas = await api('/api/personas')
  const related = (personas || []).filter((item) => String(item.datasourceId) === String(route.params.id))
  await Promise.all(related.map((item) => api(`/api/personas/${item.id}/catalog/rebuild`, { method: 'POST' })))
  ElMessage.success('目录已更新')
}

async function saveTable() {
  saving.value = true
  try {
    await api(`/api/datasources/${route.params.id}/tables/${encodeURIComponent(selected.value)}/meta`, {
      method: 'PUT',
      body: { ...tableForm.value },
    })
    await loadTables()
    await rebuildCatalogs()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    saving.value = false
  }
}

async function saveColumn(row) {
  saving.value = true
  try {
    await api(`/api/datasources/${route.params.id}/tables/${encodeURIComponent(selected.value)}/columns/${encodeURIComponent(row.name)}/meta`, {
      method: 'PUT',
      body: { comment: row.comment || '', description: row.description || '', sensitive: !!row.sensitive },
    })
    await selectTable(selected.value)
    await rebuildCatalogs()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    await loadTables()
  } catch (err) {
    ElMessage.error(err.message)
  }
})

watch(() => route.params.id, loadTables)
</script>

<template>
  <Shell>
    <template #side>
      <AdminNav />
    </template>
    <div class="page">
      <div class="page-head">
        <h1>表和字段</h1>
      </div>
      <p class="hint">隐藏的表不能授权。敏感字段不会进入模型目录，预览显示为 ***。保存注释后会重建相关人物的目录。</p>
      <div class="split">
        <div class="table-list">
          <button
            v-for="item in tables"
            :key="item.name"
            type="button"
            class="nav-btn pick-btn"
            :class="{ on: item.name === selected }"
            @click="selectTable(item.name)"
          >
            {{ item.name }}<span v-if="item.hidden">（隐藏）</span>
          </button>
        </div>
        <div v-if="selected">
          <el-form label-position="top" class="stack-form">
            <el-form-item label="表注释">
              <el-input v-model="tableForm.comment" />
            </el-form-item>
            <el-form-item label="表说明">
              <el-input v-model="tableForm.description" type="textarea" :rows="2" />
            </el-form-item>
            <el-form-item label="隐藏">
              <el-switch v-model="tableForm.hidden" />
            </el-form-item>
            <el-button type="primary" :loading="saving" @click="saveTable">保存表</el-button>
          </el-form>
          <h2>字段</h2>
          <el-table :data="columns">
            <el-table-column prop="name" label="字段" width="140" />
            <el-table-column prop="type" label="类型" width="120" />
            <el-table-column label="注释">
              <template #default="{ row }"><el-input v-model="row.comment" /></template>
            </el-table-column>
            <el-table-column label="说明">
              <template #default="{ row }"><el-input v-model="row.description" /></template>
            </el-table-column>
            <el-table-column label="敏感" width="80">
              <template #default="{ row }"><el-switch v-model="row.sensitive" /></template>
            </el-table-column>
            <el-table-column width="90">
              <template #default="{ row }">
                <el-button link type="primary" :disabled="saving" @click="saveColumn(row)">保存</el-button>
              </template>
            </el-table-column>
          </el-table>
          <h2>预览</h2>
          <el-table :data="preview.rows">
            <el-table-column v-for="(col, index) in preview.columns" :key="col" :label="col">
              <template #default="{ row }">{{ row[index] }}</template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </div>
  </Shell>
</template>
