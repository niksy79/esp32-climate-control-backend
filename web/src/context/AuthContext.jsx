import { createContext, useContext, useState, useCallback } from 'react'
import { setOnTokenRefreshed } from '../api/client'

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [token, setToken] = useState(() => localStorage.getItem('access_token'))

  const onTokenRefreshed = useCallback((newToken) => {
    setToken(newToken)
  }, [])

  // Register the callback so the axios interceptor can sync React state
  setOnTokenRefreshed(onTokenRefreshed)

  function login(newToken) {
    localStorage.setItem('access_token', newToken)
    setToken(newToken)
  }

  function logout() {
    localStorage.removeItem('access_token')
    localStorage.removeItem('refresh_token')
    setToken(null)
  }

  return (
    <AuthContext.Provider value={{ token, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  return useContext(AuthContext)
}
