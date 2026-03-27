import { useState } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { decodeToken } from '../utils/index'
import './BottomNav.css'

function IconHome({ active }) {
  const c = active ? '#4fc3f7' : 'rgba(255,255,255,0.45)'
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 9.5L12 3l9 6.5V20a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9.5z" />
      <path d="M9 21V12h6v9" />
    </svg>
  )
}

function IconUsers({ active }) {
  const c = active ? '#4fc3f7' : 'rgba(255,255,255,0.45)'
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="9" cy="7" r="4" />
      <path d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2" />
      <path d="M16 3.13a4 4 0 0 1 0 7.75" />
      <path d="M21 21v-2a4 4 0 0 0-3-3.87" />
    </svg>
  )
}

function IconBell({ active }) {
  const c = active ? '#4fc3f7' : 'rgba(255,255,255,0.45)'
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
      <path d="M13.73 21a2 2 0 0 1-3.46 0" />
    </svg>
  )
}

function IconGear({ active }) {
  const c = active ? '#4fc3f7' : 'rgba(255,255,255,0.45)'
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  )
}

function IconPerson({ active }) {
  const c = active ? '#4fc3f7' : 'rgba(255,255,255,0.45)'
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="8" r="4" />
      <path d="M4 20c0-4 3.6-7 8-7s8 3 8 7" />
    </svg>
  )
}

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
  function isActive(path) { return location.pathname === path }

  return (
    <>
      {toast && <div className="bottom-toast">{toast}</div>}

      <div className="bottom-nav-wrapper">
        <nav className="bottom-nav">
          <button className={nav('/')} onClick={() => navigate('/')}>
            <span className="bottom-nav-icon"><IconHome active={isActive('/')} /></span>
            <span className="bottom-nav-label">Начало</span>
          </button>

          {isAdmin && (
            <button className={nav('/users')} onClick={() => navigate('/users')}>
              <span className="bottom-nav-icon"><IconUsers active={isActive('/users')} /></span>
              <span className="bottom-nav-label">Потребители</span>
            </button>
          )}

          <button className="bottom-nav-item" onClick={showComingSoon}>
            <span className="bottom-nav-icon"><IconBell active={false} /></span>
            <span className="bottom-nav-label">Известия</span>
          </button>

          <button className="bottom-nav-item" onClick={showComingSoon}>
            <span className="bottom-nav-icon"><IconGear active={false} /></span>
            <span className="bottom-nav-label">Настройки</span>
          </button>

          <button className={nav('/profile')} onClick={() => navigate('/profile')}>
            <span className="bottom-nav-icon"><IconPerson active={isActive('/profile')} /></span>
            <span className="bottom-nav-label">Профил</span>
          </button>
        </nav>
      </div>
    </>
  )
}
