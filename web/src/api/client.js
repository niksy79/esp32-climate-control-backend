import axios from 'axios'

const client = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '',
})

client.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

function logout() {
  localStorage.removeItem('access_token')
  localStorage.removeItem('refresh_token')
  window.location.href = '/login'
}

client.interceptors.response.use(
  (response) => response,
  async (error) => {
    const original = error.config

    // Not a 401, or already a retry, or this IS an auth call — don't loop
    if (
      error.response?.status !== 401 ||
      original._retry ||
      original.url === '/api/auth/refresh' ||
      original.url === '/api/auth/login' ||
      original.url === '/api/auth/register'
    ) {
      return Promise.reject(error)
    }

    const refreshToken = localStorage.getItem('refresh_token')
    if (!refreshToken) {
      logout()
      return Promise.reject(error)
    }

    original._retry = true
    try {
      const { data } = await client.post('/api/auth/refresh', { token: refreshToken })
      localStorage.setItem('access_token', data.access_token)
      localStorage.setItem('refresh_token', data.refresh_token)
      original.headers.Authorization = `Bearer ${data.access_token}`
      return client(original)
    } catch {
      logout()
      return Promise.reject(error)
    }
  }
)

export default client
