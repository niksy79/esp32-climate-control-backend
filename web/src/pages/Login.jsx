import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { login as apiLogin } from '../api/index'
import './Login.css'

export default function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()

  const [fields, setFields] = useState({ tenant_id: '', email: '', password: '' })
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  function handleChange(e) {
    setFields((prev) => ({ ...prev, [e.target.name]: e.target.value }))
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const { data } = await apiLogin(fields)
      login(data.access_token)
      localStorage.setItem('refresh_token', data.refresh_token)
      navigate('/')
    } catch (err) {
      const msg = err.response?.data?.error || 'Login failed'
      setError(msg)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-bg">
      <div className="login-card">
        <h1 className="login-title">Climate Controller</h1>
        <p className="login-subtitle">Sign in to your account</p>
        <form onSubmit={handleSubmit} className="login-form" noValidate>
          <label className="login-label" htmlFor="tenant_id">
            Tenant ID
          </label>
          <input
            id="tenant_id"
            name="tenant_id"
            type="text"
            autoComplete="organization"
            required
            className="login-input"
            value={fields.tenant_id}
            onChange={handleChange}
          />

          <label className="login-label" htmlFor="email">
            Email
          </label>
          <input
            id="email"
            name="email"
            type="email"
            autoComplete="email"
            required
            className="login-input"
            value={fields.email}
            onChange={handleChange}
          />

          <label className="login-label" htmlFor="password">
            Password
          </label>
          <input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
            className="login-input"
            value={fields.password}
            onChange={handleChange}
          />

          {error && <p className="login-error">{error}</p>}

          <button type="submit" className="login-btn" disabled={loading}>
            {loading ? 'Signing in...' : 'Sign In'}
          </button>
        </form>
      </div>
    </div>
  )
}
