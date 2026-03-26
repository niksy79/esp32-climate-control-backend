import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { forgotPassword } from '../api/index'
import './Login.css'

export default function ForgotPassword() {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [sent, setSent] = useState(false)
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e) {
    e.preventDefault()
    setLoading(true)
    try {
      await forgotPassword(email)
    } finally {
      setLoading(false)
      setSent(true) // Винаги показваме success за да не разкрием дали email съществува
    }
  }

  if (sent) {
    return (
      <div className="login-page">
        <div className="login-card">
          <h1 className="login-title">Проверете имейла си</h1>
          <p style={{ color: '#94a3b8', textAlign: 'center', marginBottom: '24px' }}>
            Ако акаунтът съществува, ще получите линк за смяна на паролата.
          </p>
          <button className="login-btn" onClick={() => navigate('/login')}>
            Назад към вход
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <h1 className="login-title">Забравена парола</h1>
        <form onSubmit={handleSubmit} className="login-form">
          <div className="login-field">
            <label className="login-label">Имейл</label>
            <input className="login-input" type="email" required
              value={email} onChange={e => setEmail(e.target.value)} />
          </div>
          <button className="login-btn" type="submit" disabled={loading}>
            {loading ? 'Изпращане...' : 'Изпрати линк'}
          </button>
          <div className="login-links">
            <button type="button" className="login-link-btn" onClick={() => navigate('/login')}>
              Назад към вход
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
