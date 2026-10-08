<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { api } from '../api'
import { useSession } from '../stores/session'
import Mark from '../components/Mark.vue'

const router = useRouter()
const session = useSession()
const tab = ref('login')
const busy = ref(false)
const loginForm = reactive({ username: '', password: '' })
const registerForm = reactive({ username: '', password: '', displayName: '', dept: '' })

async function submitLogin() {
  busy.value = true
  try {
    const data = await api('/api/auth/login', { method: 'POST', body: { ...loginForm } })
    session.apply(data)
    router.push('/')
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    busy.value = false
  }
}

async function submitRegister() {
  busy.value = true
  try {
    const data = await api('/api/auth/register', { method: 'POST', body: { ...registerForm } })
    session.apply(data)
    ElMessage.success('注册成功')
    router.push('/')
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <section class="login-hero">
      <div class="lockup">
        <Mark />
        <span>数据分析</span>
      </div>
      <div>
        <h1>把业务库问清楚</h1>
        <p class="hero-copy">选择一个人物，用中文提问。系统只在授权表上做只读查询，并给出 SQL 依据。</p>
        <ul>
          <li>只读打开 SQLite 文件</li>
          <li>按人物限制表和行级条件</li>
          <li>没有查到就不下结论</li>
        </ul>
      </div>
    </section>
    <section class="login-panel">
      <div class="login-card">
        <h2>{{ tab === 'login' ? '登录' : '注册' }}</h2>
        <p class="sub">{{ tab === 'login' ? '使用已有账号进入问答。' : '新用户角色为 USER。' }}</p>
        <div class="segment" role="tablist">
          <button type="button" role="tab" :class="{ on: tab === 'login' }" :aria-selected="tab === 'login'" @click="tab = 'login'">登录</button>
          <button type="button" role="tab" :class="{ on: tab === 'register' }" :aria-selected="tab === 'register'" @click="tab = 'register'">注册</button>
        </div>
        <el-form v-if="tab === 'login'" label-position="top" @submit.prevent="submitLogin">
          <el-form-item label="用户名">
            <el-input v-model="loginForm.username" size="large" autocomplete="username" />
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="loginForm.password" size="large" type="password" show-password autocomplete="current-password" />
          </el-form-item>
          <el-button type="primary" native-type="submit" size="large" :loading="busy" @click="submitLogin">登录</el-button>
        </el-form>
        <el-form v-else label-position="top" @submit.prevent="submitRegister">
          <el-form-item label="用户名">
            <el-input v-model="registerForm.username" size="large" autocomplete="username" />
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="registerForm.password" size="large" type="password" show-password autocomplete="new-password" />
          </el-form-item>
          <el-form-item label="显示名">
            <el-input v-model="registerForm.displayName" size="large" />
          </el-form-item>
          <el-form-item label="部门">
            <el-input v-model="registerForm.dept" size="large" />
          </el-form-item>
          <p class="hint">用户名 3 到 32 个字符，密码至少 8 位。新用户角色为 USER。</p>
          <el-button type="primary" native-type="submit" size="large" :loading="busy" @click="submitRegister">注册</el-button>
        </el-form>
      </div>
    </section>
  </div>
</template>
