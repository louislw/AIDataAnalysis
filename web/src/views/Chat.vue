<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
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
const thread = ref(null)

const personaId = computed(() => route.params.personaId)
const persona = computed(() => personas.value.find((item) => String(item.id) === String(personaId.value)))

watch(personaId, () => {
  sessionId.value = ''
  messages.value = []
  title.value = '新对话'
  load()
}, { immediate: true })

watch([messages, busy], async () => {
  await nextTick()
  const el = thread.value
  if (el) el.scrollTop = el.scrollHeight
}, { deep: true })

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
    await ElMessageBox.confirm('删除后无法恢复。', '删除会话', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonType: 'danger',
    })
  } catch {
    return
  }
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
      <p v-if="!personas.length" class="side-empty">没有可用人物</p>
      <button
        v-for="item in personas"
        :key="item.id"
        type="button"
        class="nav-btn"
        :class="{ on: String(item.id) === String(personaId) }"
        :title="item.name"
        @click="openPersona(item.id)"
      >
        <span class="nav-text">{{ item.name }}</span>
      </button>
      <div class="side-label">会话</div>
      <button type="button" class="nav-btn new" :class="{ on: !sessionId }" @click="newChat">新对话</button>
      <div
        v-for="item in sessions"
        :key="item.id"
        class="nav-row"
        :class="{ on: item.id === sessionId }"
      >
        <button type="button" class="nav-btn" :title="item.title" @click="openSession(item)">
          <span class="nav-text">{{ item.title }}</span>
        </button>
        <button type="button" class="icon-btn" aria-label="删除会话" title="删除会话" @click="removeSession(item)">
          <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
            <path d="M4 4.5h8M6.2 4.4V3.4h3.6v1M5.2 4.5l.45 8h4.7l.45-8" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
      </div>
    </template>
    <section class="chat-col">
      <header class="chat-head">
        <div class="chat-head-inner">
          <div>
            <p class="kicker">{{ persona ? '当前人物' : '问答' }}</p>
            <h1>{{ persona ? persona.name : '人物不可用' }}</h1>
            <p class="hint">{{ title }}</p>
          </div>
          <el-button v-if="sessionId" @click="removeSession({ id: sessionId })">删除会话</el-button>
        </div>
      </header>
      <div ref="thread" class="messages">
        <div class="thread">
          <div v-if="!persona" class="chat-empty">
            <h2>人物不可用</h2>
            <p>当前账号不能使用这个人物。</p>
          </div>
          <div v-else-if="!messages.length && !busy" class="chat-empty">
            <h2>向「{{ persona.name }}」提问</h2>
            <p>回答会附上执行的 SQL、涉及的表和返回行数。没有查到数据时，只会澄清或说明失败原因。</p>
          </div>
          <article v-for="(item, index) in messages" :key="item.id || index" class="msg" :class="item.role === 'USER' ? 'user' : 'assistant'">
            <div class="who">{{ item.role === 'USER' ? '我' : '助手' }}</div>
            <div v-if="item.role === 'USER'" class="bubble">{{ item.content }}</div>
            <div v-else class="answer-card" :class="item.kind === 'clarify' ? 'kind-clarify' : item.kind === 'error' ? 'kind-error' : 'kind-answer'">
              <el-tag v-if="item.kind === 'clarify'" type="warning">需要澄清</el-tag>
              <el-tag v-else-if="item.kind === 'error'" type="danger">失败</el-tag>
              <el-tag v-else type="success">结果</el-tag>
              <div class="md" v-html="renderMarkdown(item.content)"></div>
              <div v-if="item.kind === 'answer'" class="evidence">
                <div class="ev-title">依据</div>
                <div class="ev-label">SQL</div>
                <pre>{{ item.sql }}</pre>
                <div class="ev-meta">
                  <span>表：{{ (item.tables || []).join('、') || '无' }}</span>
                  <span>行数：{{ item.rowCount ?? 0 }}</span>
                </div>
                <p v-if="item.truncated" class="trunc">结果已截断。这是返回的明细，不是全量。</p>
                <el-collapse>
                  <el-collapse-item title="分析步骤" name="steps">
                    <pre>{{ item.reasoning || '无' }}</pre>
                  </el-collapse-item>
                </el-collapse>
              </div>
            </div>
          </article>
          <article v-if="busy" class="msg assistant">
            <div class="who">助手</div>
            <div class="pending">正在查询授权范围内的数据…</div>
          </article>
        </div>
      </div>
      <form class="composer" @submit.prevent="send">
        <div class="composer-box">
          <el-input
            v-model="question"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
            :disabled="busy || !persona"
            placeholder="输入问题，Enter 发送，Shift+Enter 换行"
            @keydown="onKeydown"
          />
          <el-button type="primary" native-type="submit" :disabled="busy || !question.trim() || !persona" :loading="busy">
            发送
          </el-button>
        </div>
      </form>
    </section>
  </Shell>
</template>
