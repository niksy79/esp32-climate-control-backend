import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { resetPassword } from '../api/index'
import './Login.css'

export default function ResetPassword() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const [form, setForm] = useState({ password: '', confirm: '' })
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)
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
      await resetPassword(token, form.password)
      setDone(true)
    } catch (err) {
      setError(err.response?.data?.error || 'Невалиден или изтекъл линк')
    } finally {
      setLoading(false)
    }
  }

  if (!token) {
    return (
      <div className="login-page">
        <div className="login-card">
          <p className="login-error">Невалиден линк за смяна на парола.</p>
          <button className="login-btn" onClick={() => navigate('/login')}>Назад</button>
        </div>
      </div>
    )
  }

  if (done) {
    return (
      <div className="login-page">
        <div className="login-card">
          <h1 className="login-title">Паролата е сменена</h1>
          <p style={{ color: '#94a3b8', textAlign: 'center', marginBottom: '24px' }}>
            Можете да влезете с новата си парола.
          </p>
          <button className="login-btn" onClick={() => navigate('/login')}>
            Към вход
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <h1 className="login-title">Нова парола</h1>
        <form onSubmit={handleSubmit} className="login-form">
          <div className="login-field">
            <label className="login-label">Нова парола</label>
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
            {loading ? 'Запазване...' : 'Смени паролата'}
          </button>
        </form>
      </div>
    </div>
  )
}
