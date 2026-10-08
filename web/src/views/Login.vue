<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { api } from '../api'
import { useSession } from '../stores/session'

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
    <el-card class="login-card">
      <h1>数据分析</h1>
      <el-tabs v-model="tab">
        <el-tab-pane label="登录" name="login">
          <el-form label-position="top" @submit.prevent="submitLogin">
            <el-form-item label="用户名">
              <el-input v-model="loginForm.username" autocomplete="username" />
            </el-form-item>
            <el-form-item label="密码">
              <el-input v-model="loginForm.password" type="password" show-password autocomplete="current-password" />
            </el-form-item>
            <el-button type="primary" :loading="busy" @click="submitLogin">登录</el-button>
          </el-form>
        </el-tab-pane>
        <el-tab-pane label="注册" name="register">
          <el-form label-position="top" @submit.prevent="submitRegister">
            <el-form-item label="用户名">
              <el-input v-model="registerForm.username" autocomplete="username" />
            </el-form-item>
            <el-form-item label="密码">
              <el-input v-model="registerForm.password" type="password" show-password autocomplete="new-password" />
            </el-form-item>
            <el-form-item label="显示名">
              <el-input v-model="registerForm.displayName" />
            </el-form-item>
            <el-form-item label="部门">
              <el-input v-model="registerForm.dept" />
            </el-form-item>
            <p class="hint">用户名 3 到 32 个字符，密码至少 8 位。新用户角色为 USER。</p>
            <el-button type="primary" :loading="busy" @click="submitRegister">注册</el-button>
          </el-form>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>
