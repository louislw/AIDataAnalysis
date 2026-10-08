import { useSession } from './stores/session'

export class ApiError extends Error {
  constructor(status, message) {
    super(message)
    this.status = status
  }
}

export async function api(path, { method = 'GET', body, timeoutMs } = {}) {
  const session = useSession()
  const headers = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (session.token) headers.Authorization = `Bearer ${session.token}`
  let response
  try {
    response = await fetch(path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: timeoutMs ? AbortSignal.timeout(timeoutMs) : undefined,
    })
  } catch (err) {
    if (err && (err.name === 'TimeoutError' || err.name === 'AbortError')) {
      throw new ApiError(0, '请求超时，请稍后重试')
    }
    throw new ApiError(0, '网络请求失败')
  }
  if (response.status === 204) return null
  const text = await response.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { error: text }
    }
  }
  if (!response.ok) {
    if (response.status === 401 && session.token) {
      session.clear()
      if (!location.pathname.startsWith('/login')) location.assign('/login')
    }
    throw new ApiError(response.status, (data && data.error) || '请求失败')
  }
  return data
}
