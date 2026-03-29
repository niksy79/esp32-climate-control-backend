import { useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { AuthProvider } from './context/AuthContext'
import { useAuth } from './context/AuthContext'
import { decodeToken } from './utils/index'
import Dashboard from './pages/Dashboard'
import DeviceDetail from './pages/DeviceDetail'
import Login from './pages/Login'
import Register from './pages/Register'
import Profile from './pages/Profile'
import Users from './pages/Users'
import ForgotPassword from './pages/ForgotPassword'
import ResetPassword from './pages/ResetPassword'
import Sidebar from './components/Sidebar'

function PrivateRoute({ children }) {
  const { token } = useAuth()
  return token ? children : <Navigate to="/login" replace />
}

function PublicOnlyRoute({ children }) {
  const { token } = useAuth()
  return token ? <Navigate to="/" replace /> : children
}

function HamburgerIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
      <line x1="3" y1="6" x2="21" y2="6" />
      <line x1="3" y1="12" x2="21" y2="12" />
      <line x1="3" y1="18" x2="21" y2="18" />
    </svg>
  )
}

const PAGE_TITLES = {
  '/':        'Устройства',
  '/profile': 'Профил',
  '/users':   'Потребители',
}

function AppHeader() {
  const { token } = useAuth()
  const location = useLocation()
  const title = PAGE_TITLES[location.pathname] ?? null
  const claims = token ? decodeToken(token) : null
  const isAdmin = claims?.role === 'admin'

  // Device detail pages have their own header
  if (location.pathname.startsWith('/device/')) return null
  if (!title) return null

  return (
    <header className="app-header">
      <h2 className="page-title">{title}</h2>
      {location.pathname === '/' && isAdmin && (
        <button className="add-device-btn" title="Добави устройство">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
            <line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" />
          </svg>
          Добави
        </button>
      )}
    </header>
  )
}

function AppShell() {
  const { token } = useAuth()
  const [sidebarOpen, setSidebarOpen] = useState(false)

  return (
    <>
      {token && (
        <>
          <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />
          <button className="sidebar-toggle" onClick={() => setSidebarOpen(true)}>
            <HamburgerIcon />
          </button>
        </>
      )}

      <div className={token ? 'app-layout' : ''}>
        <div className={token ? 'app-content' : ''}>
          {token && <AppHeader />}
          <Routes>
            <Route path="/" element={<PrivateRoute><Dashboard /></PrivateRoute>} />
            <Route path="/device/:id" element={<PrivateRoute><DeviceDetail /></PrivateRoute>} />
            <Route path="/profile" element={<PrivateRoute><Profile /></PrivateRoute>} />
            <Route path="/users" element={<PrivateRoute><Users /></PrivateRoute>} />
            <Route path="/login" element={<PublicOnlyRoute><Login /></PublicOnlyRoute>} />
            <Route path="/register" element={<PublicOnlyRoute><Register /></PublicOnlyRoute>} />
            <Route path="/forgot-password" element={<PublicOnlyRoute><ForgotPassword /></PublicOnlyRoute>} />
            <Route path="/reset-password" element={<PublicOnlyRoute><ResetPassword /></PublicOnlyRoute>} />
          </Routes>
        </div>
      </div>
    </>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <AppShell />
      </BrowserRouter>
    </AuthProvider>
  )
}
