<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import Shell from '../components/Shell.vue'
import AdminNav from '../components/AdminNav.vue'
import { api } from '../api'

const route = useRoute()
const tab = ref('profile')
const deptHelp = '行级条件里的 {{user.dept}} 取这里的部门。'
const filterHelp = '只允许列比较、AND、OR 和括号。值使用占位符，不要加引号：dept_id = {{user.dept}}。可用 {{user.dept}}、{{user.id}}、{{user.username}}、{{user.display_name}}。'
const sources = ref([])
const users = ref([])
const knownTables = ref([])
const saving = ref(false)
const persona = reactive({
  name: '',
  description: '',
  systemPrompt: '',
  prohibitions: '',
  rowFilter: '',
  datasourceId: 0,
  status: 1,
  catalogText: '',
})
const grants = reactive({ tables: [], roles: [], userIds: [] })
const metrics = ref([])
const examples = ref([])

function payload() {
  return {
    name: persona.name.trim(),
    description: persona.description,
    systemPrompt: persona.systemPrompt,
    prohibitions: persona.prohibitions,
    rowFilter: persona.rowFilter,
    datasourceId: persona.datasourceId || 0,
    status: persona.status,
  }
}

async function load() {
  const [detail, sourceList, userList] = await Promise.all([
    api(`/api/personas/${route.params.id}`),
    api('/api/datasources'),
    api('/api/auth/users'),
  ])
  Object.assign(persona, detail.persona)
  grants.tables = detail.tables || []
  grants.roles = detail.roles || []
  grants.userIds = detail.userIds || []
  metrics.value = detail.metrics || []
  examples.value = detail.examples || []
  sources.value = sourceList || []
  users.value = userList || []
  await loadTables()
}

async function loadTables() {
  knownTables.value = []
  if (!persona.datasourceId) return
  const data = await api(`/api/datasources/${persona.datasourceId}/tables`)
  knownTables.value = (data.tables || []).filter((item) => !item.hidden)
}

async function saveProfile() {
  saving.value = true
  try {
    const updated = await api(`/api/personas/${route.params.id}`, { method: 'PUT', body: payload() })
    Object.assign(persona, updated)
    await loadTables()
    await rebuild()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    saving.value = false
  }
}

async function saveFilter() {
  saving.value = true
  try {
    const updated = await api(`/api/personas/${route.params.id}`, { method: 'PUT', body: payload() })
    Object.assign(persona, updated)
    ElMessage.success('已保存')
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    saving.value = false
  }
}

async function saveGrants() {
  saving.value = true
  try {
    await api(`/api/personas/${route.params.id}/tables`, { method: 'PUT', body: { tables: grants.tables } })
    await api(`/api/personas/${route.params.id}/roles`, { method: 'PUT', body: { roles: grants.roles } })
    await api(`/api/personas/${route.params.id}/users`, { method: 'PUT', body: { userIds: grants.userIds } })
    await rebuild()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    saving.value = false
  }
}

async function saveKnowledge() {
  saving.value = true
  try {
    const metricBody = metrics.value
      .filter((item) => item.name.trim() && item.definition.trim())
      .map((item, index) => ({ name: item.name.trim(), definition: item.definition.trim(), sortOrder: index }))
    const exampleBody = examples.value
      .filter((item) => item.question.trim() && item.sql.trim())
      .map((item, index) => ({ question: item.question.trim(), sql: item.sql.trim(), sortOrder: index }))
    await api(`/api/personas/${route.params.id}/metrics`, { method: 'PUT', body: { metrics: metricBody } })
    await api(`/api/personas/${route.params.id}/examples`, { method: 'PUT', body: { examples: exampleBody } })
    metrics.value = metricBody
    examples.value = exampleBody
    await rebuild()
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    saving.value = false
  }
}

async function saveUser(row) {
  try {
    await api(`/api/auth/users/${row.id}`, {
      method: 'PUT',
      body: { role: row.role, displayName: row.displayName, dept: row.dept, enabled: row.enabled },
    })
    ElMessage.success('用户已更新')
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function rebuild() {
  const data = await api(`/api/personas/${route.params.id}/catalog/rebuild`, { method: 'POST' })
  persona.catalogText = data.catalogText || ''
  ElMessage.success('目录已更新')
}

onMounted(async () => {
  try {
    await load()
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
        <h1>{{ persona.name || '人物' }}</h1>
      </div>
      <el-tabs v-model="tab">
        <el-tab-pane label="设定" name="profile">
          <el-form label-position="top" class="stack-form">
            <el-form-item label="名称"><el-input v-model="persona.name" /></el-form-item>
            <el-form-item label="说明"><el-input v-model="persona.description" type="textarea" :rows="2" /></el-form-item>
            <el-form-item label="人物设定"><el-input v-model="persona.systemPrompt" type="textarea" :rows="4" /></el-form-item>
            <el-form-item label="禁止事项"><el-input v-model="persona.prohibitions" type="textarea" :rows="3" /></el-form-item>
            <el-form-item label="数据源">
              <el-select v-model="persona.datasourceId" clearable style="width:100%" @change="loadTables">
                <el-option v-for="item in sources" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="启用"><el-switch v-model="persona.status" :active-value="1" :inactive-value="0" /></el-form-item>
            <el-button type="primary" :loading="saving" @click="saveProfile">保存设定</el-button>
          </el-form>
          <h2>当前目录</h2>
          <pre class="evidence">{{ persona.catalogText || '尚未生成' }}</pre>
        </el-tab-pane>
        <el-tab-pane label="授权" name="grants">
          <p v-if="!persona.datasourceId" class="hint">先在设定里选择数据源。</p>
          <template v-else>
            <p v-if="grants.tables.length === 0" class="empty-copy">未授权任何表，该人物不能查询</p>
            <el-checkbox-group v-model="grants.tables">
              <el-checkbox v-for="item in knownTables" :key="item.name" :value="item.name">{{ item.name }}</el-checkbox>
            </el-checkbox-group>
            <p class="hint">隐藏的表不会出现在列表里，也不能授权。</p>
            <h2>角色</h2>
            <el-checkbox-group v-model="grants.roles">
              <el-checkbox value="ADMIN">ADMIN</el-checkbox>
              <el-checkbox value="USER">USER</el-checkbox>
            </el-checkbox-group>
            <p class="hint">管理员使用人物时跳过角色和用户授权。其他用户需要角色匹配；勾选用户后，只有这些用户能使用。</p>
            <h2>用户</h2>
            <el-checkbox-group v-model="grants.userIds">
              <el-checkbox v-for="user in users" :key="user.id" :value="user.id">{{ user.username }}（{{ user.dept || '无部门' }}）</el-checkbox>
            </el-checkbox-group>
            <div style="margin-top:12px">
              <el-button type="primary" :loading="saving" @click="saveGrants">保存授权</el-button>
            </div>
            <h2>部门</h2>
            <p class="hint">{{ deptHelp }}</p>
            <el-table :data="users">
              <el-table-column prop="username" label="用户" width="140" />
              <el-table-column label="角色" width="140">
                <template #default="{ row }">
                  <el-select v-model="row.role">
                    <el-option label="ADMIN" value="ADMIN" />
                    <el-option label="USER" value="USER" />
                  </el-select>
                </template>
              </el-table-column>
              <el-table-column label="部门">
                <template #default="{ row }"><el-input v-model="row.dept" /></template>
              </el-table-column>
              <el-table-column label="启用" width="80">
                <template #default="{ row }"><el-switch v-model="row.enabled" /></template>
              </el-table-column>
              <el-table-column width="90">
                <template #default="{ row }"><el-button link type="primary" @click="saveUser(row)">保存</el-button></template>
              </el-table-column>
            </el-table>
          </template>
        </el-tab-pane>
        <el-tab-pane label="口径与示例" name="knowledge">
          <h2>口径</h2>
          <div v-for="(item, index) in metrics" :key="'m' + index" class="stack-form">
            <el-input v-model="item.name" placeholder="名称" />
            <el-input v-model="item.definition" type="textarea" :rows="2" placeholder="定义" style="margin:8px 0" />
          </div>
          <el-button @click="metrics.push({ name: '', definition: '', sortOrder: metrics.length })">添加口径</el-button>
          <h2>示例</h2>
          <div v-for="(item, index) in examples" :key="'e' + index" class="stack-form">
            <el-input v-model="item.question" placeholder="问题" />
            <el-input v-model="item.sql" type="textarea" :rows="2" placeholder="SQL" style="margin:8px 0" />
          </div>
          <el-button @click="examples.push({ question: '', sql: '', sortOrder: examples.length })">添加示例</el-button>
          <div style="margin-top:12px">
            <el-button type="primary" :loading="saving" @click="saveKnowledge">保存口径与示例</el-button>
          </div>
        </el-tab-pane>
        <el-tab-pane label="行级条件" name="filter">
          <p class="hint">{{ filterHelp }}</p>
          <el-input v-model="persona.rowFilter" type="textarea" :rows="4" class="stack-form" />
          <div style="margin-top:12px">
            <el-button type="primary" :loading="saving" @click="saveFilter">保存行级条件</el-button>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>
  </Shell>
</template>
