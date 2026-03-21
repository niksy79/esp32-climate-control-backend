import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from './context/AuthContext'
import { useAuth } from './context/AuthContext'
import Dashboard from './pages/Dashboard'
import DeviceDetail from './pages/DeviceDetail'
import Login from './pages/Login'

function PrivateRoute({ children }) {
  const { token } = useAuth()
  return token ? children : <Navigate to="/login" replace />
}

function PublicOnlyRoute({ children }) {
  const { token } = useAuth()
  return token ? <Navigate to="/" replace /> : children
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <PrivateRoute>
                <Dashboard />
              </PrivateRoute>
            }
          />
          <Route
            path="/device/:id"
            element={
              <PrivateRoute>
                <DeviceDetail />
              </PrivateRoute>
            }
          />
          <Route
            path="/login"
            element={
              <PublicOnlyRoute>
                <Login />
              </PublicOnlyRoute>
            }
          />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
