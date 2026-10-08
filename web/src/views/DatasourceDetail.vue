<script setup>
import { onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import Shell from '../components/Shell.vue'
import AdminNav from '../components/AdminNav.vue'
import { api } from '../api'

const route = useRoute()
const source = ref(null)
const tables = ref([])
const selected = ref('')
const tableForm = ref({ comment: '', description: '', hidden: false })
const columns = ref([])
const preview = ref({ columns: [], rows: [] })
const saving = ref(false)

async function loadSource() {
  const list = await api('/api/datasources')
  source.value = (list || []).find((item) => String(item.id) === String(route.params.id)) || null
}

async function loadTables() {
  const data = await api(`/api/datasources/${route.params.id}/tables`)
  tables.value = data.tables || []
  if (!tables.value.some((item) => item.name === selected.value)) selected.value = ''
  if (!selected.value && tables.value.length) await selectTable(tables.value[0].name)
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

async function reload() {
  selected.value = ''
  await Promise.all([loadSource(), loadTables()])
}

onMounted(async () => {
  try {
    await reload()
  } catch (err) {
    ElMessage.error(err.message)
  }
})

watch(() => route.params.id, async () => {
  try {
    await reload()
  } catch (err) {
    ElMessage.error(err.message)
  }
})
</script>

<template>
  <Shell>
    <template #side>
      <AdminNav />
    </template>
    <div class="page">
      <div class="page-head">
        <div>
          <router-link class="crumb" to="/admin/datasources">数据源</router-link>
          <h1>{{ source?.name || '表和字段' }}</h1>
          <p class="lede">隐藏的表不能授权。敏感字段不会进入模型目录，预览显示为 ***。保存注释后会重建相关人物的目录。</p>
          <p v-if="source?.fileName" class="hint mono">{{ source.fileName }}</p>
        </div>
      </div>
      <div class="split">
        <div class="table-list">
          <p v-if="!tables.length" class="hint">这个数据源里没有表。</p>
          <button
            v-for="item in tables"
            :key="item.name"
            type="button"
            class="pick-btn"
            :class="{ on: item.name === selected }"
            @click="selectTable(item.name)"
          >
            <span class="pick-name">{{ item.name }}</span>
            <span v-if="item.hidden" class="mini-pill">隐藏</span>
          </button>
        </div>
        <div v-if="selected">
          <div class="panel">
            <h2>{{ selected }}</h2>
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
          </div>
          <div class="panel">
            <h2>字段</h2>
            <div class="table-card">
              <el-table :data="columns">
                <el-table-column prop="name" label="字段" min-width="120" />
                <el-table-column prop="type" label="类型" width="120" />
                <el-table-column label="注释" min-width="140">
                  <template #default="{ row }"><el-input v-model="row.comment" /></template>
                </el-table-column>
                <el-table-column label="说明" min-width="180">
                  <template #default="{ row }"><el-input v-model="row.description" /></template>
                </el-table-column>
                <el-table-column label="敏感" width="80">
                  <template #default="{ row }"><el-switch v-model="row.sensitive" /></template>
                </el-table-column>
                <el-table-column label="操作" width="90">
                  <template #default="{ row }">
                    <el-button link type="primary" :disabled="saving" @click="saveColumn(row)">保存</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </div>
          <div class="panel">
            <h2>预览</h2>
            <p class="hint">最多显示返回的前若干行。敏感列以 *** 代替。</p>
            <div class="table-card">
              <el-table :data="preview.rows">
                <el-table-column v-for="(col, index) in preview.columns" :key="col" :label="col" min-width="120">
                  <template #default="{ row }">{{ row[index] }}</template>
                </el-table-column>
              </el-table>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Shell>
</template>
