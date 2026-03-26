import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { changePassword } from '../api/index'
import './Login.css'
import './Profile.css'

export default function Profile() {
  const navigate = useNavigate()
  const [form, setForm] = useState({ old_password: '', new_password: '', confirm: '' })
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [loading, setLoading] = useState(false)

  function setField(k, v) { setForm(p => ({ ...p, [k]: v })) }

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setSuccess(false)

    if (form.new_password !== form.confirm) {
      setError('Новите пароли не съвпадат')
      return
    }
    if (form.new_password.length < 8) {
      setError('Новата парола трябва да е поне 8 символа')
      return
    }

    setLoading(true)
    try {
      await changePassword(form.old_password, form.new_password)
      setSuccess(true)
      setForm({ old_password: '', new_password: '', confirm: '' })
    } catch (err) {
      setError(err.response?.data?.error || 'Грешка при смяна на паролата')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="profile-page">
      <div className="profile-card">
        <div className="profile-header">
          <button className="profile-back-btn" onClick={() => navigate('/')}>← Назад</button>
          <h1 className="login-title" style={{ margin: 0 }}>Профил</h1>
        </div>

        <h2 className="profile-section-title">Смяна на парола</h2>

        {success && (
          <div className="profile-success">
            Паролата е сменена успешно.
          </div>
        )}

        <form onSubmit={handleSubmit} className="login-form">
          <div className="login-field">
            <label className="login-label">Текуща парола</label>
            <input
              className="login-input"
              type="password"
              required
              autoComplete="current-password"
              value={form.old_password}
              onChange={e => setField('old_password', e.target.value)}
            />
          </div>
          <div className="login-field">
            <label className="login-label">Нова парола</label>
            <input
              className="login-input"
              type="password"
              required
              minLength={8}
              autoComplete="new-password"
              value={form.new_password}
              onChange={e => setField('new_password', e.target.value)}
            />
          </div>
          <div className="login-field">
            <label className="login-label">Потвърди нова парола</label>
            <input
              className="login-input"
              type="password"
              required
              autoComplete="new-password"
              value={form.confirm}
              onChange={e => setField('confirm', e.target.value)}
            />
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
