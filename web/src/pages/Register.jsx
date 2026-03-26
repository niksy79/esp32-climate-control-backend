import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { registerUser } from '../api/index'
import { useAuth } from '../context/AuthContext'
import './Login.css'

export default function Register() {
  const navigate = useNavigate()
  const { login } = useAuth()
  const [form, setForm] = useState({ email: '', password: '', confirm: '' })
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  function setField(k, v) { setForm(p => ({ ...p, [k]: v })) }

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    if (form.password !== form.confirm) {
      setError('Паролите не съвпадат')
      return
    }
    if (form.password.length < 8) {
      setError('Паролата трябва да е поне 8 символа')
      return
    }
    setLoading(true)
    try {
      const res = await registerUser({ email: form.email, password: form.password })
      login(res.data.access_token, res.data.refresh_token)
      navigate('/')
    } catch (err) {
      setError(err.response?.data?.error || 'Грешка при регистрация')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <h1 className="login-title">Регистрация</h1>
        <form onSubmit={handleSubmit} className="login-form">
          <div className="login-field">
            <label className="login-label">Имейл</label>
            <input className="login-input" type="email" required
              value={form.email} onChange={e => setField('email', e.target.value)} />
          </div>
          <div className="login-field">
            <label className="login-label">Парола</label>
            <input className="login-input" type="password" required minLength={8}
              value={form.password} onChange={e => setField('password', e.target.value)} />
          </div>
          <div className="login-field">
            <label className="login-label">Потвърди парола</label>
            <input className="login-input" type="password" required
              value={form.confirm} onChange={e => setField('confirm', e.target.value)} />
          </div>
          {error && <p className="login-error">{error}</p>}
          <button className="login-btn" type="submit" disabled={loading}>
            {loading ? 'Регистрация...' : 'Регистрирай се'}
          </button>
          <div className="login-links">
            <button type="button" className="login-link-btn" onClick={() => navigate('/login')}>
              Вече имате акаунт? Влезте
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
