import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
})

// Store token in module scope — one read at startup, not per-request
let _token: string | null = localStorage.getItem('token')

export function setToken(t: string) {
  _token = t
  localStorage.setItem('token', t)
}

export function clearToken() {
  _token = null
  localStorage.removeItem('token')
}

// Attach JWT to every request
api.interceptors.request.use((config) => {
  if (_token) config.headers.Authorization = `Bearer ${_token}`
  return config
})

// Redirect to login on 401; surface other errors.
// Auth endpoints (login, register, setup) legitimately return 401/4xx as
// error responses — do NOT redirect for those; let the view's catch block
// display the error message instead.
api.interceptors.response.use(
  (res) => res,
  (err) => {
    const url: string = err.config?.url ?? ''
    const isAuthEndpoint =
      url.includes('/auth/login') ||
      url.includes('/auth/register') ||
      url.includes('/setup/admin')

    if (err.response?.status === 401 && !isAuthEndpoint) {
      clearToken()
      // Use replace so the back button doesn't loop back to a protected route.
      window.location.replace('/login')
    }
    return Promise.reject(err)
  },
)

export default api
