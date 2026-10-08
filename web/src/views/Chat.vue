<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import Shell from '../components/Shell.vue'
import { api } from '../api'
import { renderMarkdown } from '../markdown'
import { useSession } from '../stores/session'

const route = useRoute()
const router = useRouter()
const session = useSession()
const personas = ref([])
const sessions = ref([])
const messages = ref([])
const question = ref('')
const busy = ref(false)
const sessionId = ref('')
const title = ref('新对话')

const personaId = computed(() => route.params.personaId)
const persona = computed(() => personas.value.find((item) => String(item.id) === String(personaId.value)))

watch(personaId, () => {
  sessionId.value = ''
  messages.value = []
  title.value = '新对话'
  load()
}, { immediate: true })

async function load() {
  localStorage.setItem('ada.lastPersona', String(personaId.value))
  try {
    personas.value = await api('/api/personas/mine')
    if (!persona.value) {
      sessions.value = []
      return
    }
    sessions.value = await api(`/api/personas/${personaId.value}/sessions`)
  } catch (err) {
    ElMessage.error(err.message)
  }
}

function openPersona(id) {
  router.push(`/p/${id}`)
}

async function openSession(item) {
  sessionId.value = item.id
  title.value = item.title
  try {
    const detail = await api(`/api/sessions/${item.id}`)
    messages.value = detail.messages || []
    title.value = detail.title || item.title
  } catch (err) {
    ElMessage.error(err.message)
  }
}

function newChat() {
  sessionId.value = ''
  messages.value = []
  title.value = '新对话'
  question.value = ''
}

async function removeSession(item) {
  try {
    await api(`/api/sessions/${item.id}`, { method: 'DELETE' })
    if (sessionId.value === item.id) newChat()
    sessions.value = await api(`/api/personas/${personaId.value}/sessions`)
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function send() {
  const text = question.value.trim()
  if (!text || busy.value || !persona.value) return
  busy.value = true
  try {
    if (!sessionId.value) {
      const created = await api(`/api/personas/${personaId.value}/sessions`, { method: 'POST' })
      sessionId.value = created.id
      title.value = created.title
      sessions.value = [created, ...sessions.value]
    }
    messages.value.push({ role: 'USER', content: text, kind: '', sql: '', tables: [] })
    question.value = ''
    let answer
    try {
      answer = await api(`/api/sessions/${sessionId.value}/query`, {
        method: 'POST',
        body: { question: text },
        timeoutMs: session.llmTimeoutSeconds * 1000,
      })
    } catch (err) {
      messages.value.pop()
      question.value = text
      throw err
    }
    messages.value.push({
      role: 'ASSISTANT',
      content: answer.content,
      kind: answer.kind,
      sql: answer.sql || '',
      tables: answer.tables || [],
      rowCount: answer.rowCount,
      truncated: answer.truncated,
      reasoning: answer.reasoning || '',
    })
    sessions.value = await api(`/api/personas/${personaId.value}/sessions`)
    const current = sessions.value.find((item) => item.id === sessionId.value)
    if (current) title.value = current.title
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    busy.value = false
  }
}

function onKeydown(event) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    send()
  }
}
</script>

<template>
  <Shell>
    <template #side>
      <div class="side-label">人物</div>
      <button
        v-for="item in personas"
        :key="item.id"
        type="button"
        class="nav-btn"
        :class="{ on: String(item.id) === String(personaId) }"
        @click="openPersona(item.id)"
      >
        {{ item.name }}
      </button>
      <div class="side-label">会话</div>
      <button type="button" class="nav-btn" :class="{ on: !sessionId }" @click="newChat">新对话</button>
      <button
        v-for="item in sessions"
        :key="item.id"
        type="button"
        class="nav-btn"
        :class="{ on: item.id === sessionId }"
        @click="openSession(item)"
      >
        {{ item.title }}
      </button>
    </template>
    <section class="chat-col">
      <header class="chat-head">
        <h1>{{ persona ? persona.name : '人物不可用' }}</h1>
        <p class="hint">{{ title }}<button v-if="sessionId" class="linkish" type="button" @click="removeSession({ id: sessionId })">删除会话</button></p>
      </header>
      <div class="messages">
        <p v-if="!persona" class="hint">当前账号不能使用这个人物。</p>
        <article v-for="(item, index) in messages" :key="item.id || index" class="msg">
          <div class="who">{{ item.role === 'USER' ? '我' : '助手' }}</div>
          <div v-if="item.role === 'USER'" class="bubble">{{ item.content }}</div>
          <template v-else>
            <el-tag v-if="item.kind === 'clarify'" type="warning">需要澄清</el-tag>
            <el-tag v-else-if="item.kind === 'error'" type="danger">失败</el-tag>
            <el-tag v-else type="success">结果</el-tag>
            <div class="md" v-html="renderMarkdown(item.content)"></div>
            <div v-if="item.kind === 'answer'" class="evidence">
              <div class="ev-title">依据</div>
              <div>SQL</div>
              <pre>{{ item.sql }}</pre>
              <div>表：{{ (item.tables || []).join('、') || '无' }}</div>
              <div>行数：{{ item.rowCount ?? 0 }}</div>
              <div v-if="item.truncated">结果已截断。这是返回的明细，不是全量。</div>
              <el-collapse>
                <el-collapse-item title="分析步骤" name="steps">
                  <pre>{{ item.reasoning || '无' }}</pre>
                </el-collapse-item>
              </el-collapse>
            </div>
          </template>
        </article>
      </div>
      <form class="composer" @submit.prevent="send">
        <el-input
          v-model="question"
          type="textarea"
          :rows="3"
          :disabled="busy || !persona"
          placeholder="输入问题，Enter 发送，Shift+Enter 换行"
          @keydown="onKeydown"
        />
        <el-button type="primary" native-type="submit" :disabled="busy || !question.trim() || !persona" :loading="busy">
          发送
        </el-button>
      </form>
    </section>
  </Shell>
</template>
