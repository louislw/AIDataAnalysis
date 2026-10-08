import { defineStore } from 'pinia'

export const useSession = defineStore('session', {
  state: () => ({
    token: localStorage.getItem('ada.token') || '',
    username: localStorage.getItem('ada.username') || '',
    role: localStorage.getItem('ada.role') || '',
    displayName: localStorage.getItem('ada.displayName') || '',
    llmTimeoutSeconds: Number(localStorage.getItem('ada.timeout') || 120),
  }),
  getters: {
    isAdmin: (state) => state.role === 'ADMIN',
  },
  actions: {
    apply(data) {
      this.token = data.token || ''
      this.username = data.username || ''
      this.role = data.role || ''
      this.displayName = data.displayName || data.username || ''
      localStorage.setItem('ada.token', this.token)
      localStorage.setItem('ada.username', this.username)
      localStorage.setItem('ada.role', this.role)
      localStorage.setItem('ada.displayName', this.displayName)
    },
    setTimeoutSeconds(seconds) {
      const n = Number(seconds)
      this.llmTimeoutSeconds = n > 0 ? n : 120
      localStorage.setItem('ada.timeout', String(this.llmTimeoutSeconds))
    },
    clear() {
      this.token = ''
      this.username = ''
      this.role = ''
      this.displayName = ''
      localStorage.removeItem('ada.token')
      localStorage.removeItem('ada.username')
      localStorage.removeItem('ada.role')
      localStorage.removeItem('ada.displayName')
    },
  },
})
