import { useState } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { decodeToken } from '../utils/index'
import './BottomNav.css'

export default function BottomNav() {
  const { token } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [toast, setToast] = useState('')

  const claims  = token ? decodeToken(token) : null
  const isAdmin = claims?.role === 'admin'

  function showComingSoon() {
    setToast('Скоро')
    setTimeout(() => setToast(''), 2000)
  }

  function nav(path) { return `bottom-nav-item${location.pathname === path ? ' bottom-nav-active' : ''}` }

  return (
    <>
      {toast && <div className="bottom-toast">{toast}</div>}

      <div className="bottom-nav-wrapper">
        <nav className="bottom-nav">
          <button className={nav('/')} onClick={() => navigate('/')}>
            <span className="bottom-nav-icon">🏠</span>
            <span className="bottom-nav-label">Начало</span>
          </button>

          {isAdmin && (
            <button className={nav('/users')} onClick={() => navigate('/users')}>
              <span className="bottom-nav-icon">👥</span>
              <span className="bottom-nav-label">Потребители</span>
            </button>
          )}

          <button className="bottom-nav-item" onClick={showComingSoon}>
            <span className="bottom-nav-icon">🔔</span>
            <span className="bottom-nav-label">Известия</span>
          </button>

          <button className="bottom-nav-item" onClick={showComingSoon}>
            <span className="bottom-nav-icon">⚙️</span>
            <span className="bottom-nav-label">Настройки</span>
          </button>

          <button className={nav('/profile')} onClick={() => navigate('/profile')}>
            <span className="bottom-nav-icon">👤</span>
            <span className="bottom-nav-label">Профил</span>
          </button>
        </nav>
      </div>
    </>
  )
}
