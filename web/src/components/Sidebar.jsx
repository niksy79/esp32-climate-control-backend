import { useState, useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { useTheme } from '../hooks/useTheme'
import { decodeToken } from '../utils/index'
import './Sidebar.css'

function IconHome({ active }) {
  const c = active ? '#4fc3f7' : 'currentColor'
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 9.5L12 3l9 6.5V20a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9.5z" />
      <path d="M9 21V12h6v9" />
    </svg>
  )
}

function IconUsers({ active }) {
  const c = active ? '#4fc3f7' : 'currentColor'
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="9" cy="7" r="4" />
      <path d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2" />
      <path d="M16 3.13a4 4 0 0 1 0 7.75" />
      <path d="M21 21v-2a4 4 0 0 0-3-3.87" />
    </svg>
  )
}

function IconBell({ active }) {
  const c = active ? '#4fc3f7' : 'currentColor'
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
      <path d="M13.73 21a2 2 0 0 1-3.46 0" />
    </svg>
  )
}

function IconGear({ active }) {
  const c = active ? '#4fc3f7' : 'currentColor'
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  )
}

function IconPerson({ active }) {
  const c = active ? '#4fc3f7' : 'currentColor'
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="8" r="4" />
      <path d="M4 20c0-4 3.6-7 8-7s8 3 8 7" />
    </svg>
  )
}

function IconLogout() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <polyline points="16 17 21 12 16 7" />
      <line x1="21" y1="12" x2="9" y2="12" />
    </svg>
  )
}

export default function Sidebar({ open, onClose }) {
  const { token, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const { theme, toggle: toggleTheme } = useTheme()
  const [toast, setToast] = useState('')

  const claims  = token ? decodeToken(token) : null
  const isAdmin = claims?.role === 'admin'

  // Close sidebar on route change (mobile)
  useEffect(() => { onClose?.() }, [location.pathname])

  function go(path) {
    navigate(path)
  }

  function showComingSoon() {
    setToast('Скоро')
    setTimeout(() => setToast(''), 2000)
  }

  const isActive = (path) => location.pathname === path

  return (
    <>
      {/* Backdrop for mobile overlay */}
      {open && <div className="sidebar-backdrop" onClick={onClose} />}

      <aside className={`sidebar${open ? ' sidebar--open' : ''}`}>
        {/* Brand */}
        <div className="sidebar-brand">
          <span className="sidebar-logo">M</span>
          <span className="sidebar-brand-text">MerakOne</span>
        </div>

        {/* Nav items */}
        <nav className="sidebar-nav">
          <button className={`sidebar-item${isActive('/') ? ' sidebar-item--active' : ''}`} onClick={() => go('/')}>
            <IconHome active={isActive('/')} />
            <span>Начало</span>
          </button>

          {isAdmin && (
            <button className={`sidebar-item${isActive('/users') ? ' sidebar-item--active' : ''}`} onClick={() => go('/users')}>
              <IconUsers active={isActive('/users')} />
              <span>Потребители</span>
            </button>
          )}

          <button className="sidebar-item" onClick={showComingSoon}>
            <IconBell active={false} />
            <span>Известия</span>
          </button>

          <button className="sidebar-item" onClick={showComingSoon}>
            <IconGear active={false} />
            <span>Настройки</span>
          </button>

          <button className={`sidebar-item${isActive('/profile') ? ' sidebar-item--active' : ''}`} onClick={() => go('/profile')}>
            <IconPerson active={isActive('/profile')} />
            <span>Профил</span>
          </button>
        </nav>

        {/* Footer */}
        <div className="sidebar-footer">
          {toast && <div className="sidebar-toast">{toast}</div>}
          <div className="sidebar-user">
            <span className="sidebar-email">{claims?.email ?? ''}</span>
          </div>
          <button className="sidebar-item" onClick={toggleTheme}>
            <span style={{ fontSize: '18px' }}>{theme === 'dark' ? '☀️' : '🌙'}</span>
            <span>{theme === 'dark' ? 'Светла тема' : 'Тъмна тема'}</span>
          </button>
          <button className="sidebar-item sidebar-item--logout" onClick={logout}>
            <IconLogout />
            <span>Изход</span>
          </button>
        </div>
      </aside>
    </>
  )
}
