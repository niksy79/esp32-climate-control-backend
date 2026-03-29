import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { decodeToken } from '../utils/index'
import { listUsers, createUser, deleteUser } from '../api/index'
import './Login.css'
import './Profile.css'
import './Users.css'

export default function Users() {
  const { token } = useAuth()
  const navigate = useNavigate()

  const claims = token ? decodeToken(token) : null
  const tenantId = claims?.tenant_id ?? null
  const currentUserId = claims?.user_id ?? null
  const isAdmin = claims?.role === 'admin'

  const [users, setUsers] = useState([])
  const [loading, setLoading] = useState(true)
  const [listError, setListError] = useState('')

  const [form, setForm] = useState({ email: '', password: '', role: 'user' })
  const [formError, setFormError] = useState('')
  const [formLoading, setFormLoading] = useState(false)

  useEffect(() => {
    if (!isAdmin) {
      navigate('/', { replace: true })
      return
    }
    fetchUsers()
  }, [isAdmin]) // eslint-disable-line react-hooks/exhaustive-deps

  async function fetchUsers() {
    try {
      const { data } = await listUsers(tenantId)
      setUsers(data ?? [])
    } catch (err) {
      setListError(err.response?.data?.error || 'Грешка при зареждане на потребителите')
    } finally {
      setLoading(false)
    }
  }

  function setField(k, v) { setForm(p => ({ ...p, [k]: v })) }

  async function handleCreate(e) {
    e.preventDefault()
    setFormError('')
    if (form.password.length < 8) {
      setFormError('Паролата трябва да е поне 8 символа')
      return
    }
    setFormLoading(true)
    try {
      const { data } = await createUser(tenantId, form)
      setUsers(prev => [...prev, data])
      setForm({ email: '', password: '', role: 'user' })
    } catch (err) {
      setFormError(err.response?.data?.error || 'Грешка при създаване на потребителя')
    } finally {
      setFormLoading(false)
    }
  }

  async function handleDelete(userId) {
    if (!window.confirm('Изтрий потребителя?')) return
    try {
      await deleteUser(tenantId, userId)
      setUsers(prev => prev.filter(u => u.id !== userId))
    } catch (err) {
      setListError(err.response?.data?.error || 'Грешка при изтриване')
    }
  }

  if (!isAdmin) return null

  return (
    <div className="users-page">
      <div className="users-container">
        {listError && <p className="users-error">{listError}</p>}

        <div className="users-card">
          <h2 className="profile-section-title">Списък</h2>
          {loading ? (
            <p className="users-state">Зареждане...</p>
          ) : users.length === 0 ? (
            <p className="users-state">Няма потребители.</p>
          ) : (
            <table className="users-table">
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Роля</th>
                  <th>Регистриран</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {users.map(u => (
                  <tr key={u.id} className={u.id === currentUserId ? 'users-row-self' : ''}>
                    <td>{u.email}</td>
                    <td>
                      <span className={`users-role users-role-${u.role}`}>{u.role}</span>
                    </td>
                    <td className="users-date">
                      {new Date(u.created_at).toLocaleDateString('bg-BG')}
                    </td>
                    <td>
                      <button
                        className="users-delete-btn"
                        disabled={u.id === currentUserId}
                        title={u.id === currentUserId ? 'Не може да изтриете собствения си акаунт' : 'Изтрий'}
                        onClick={() => handleDelete(u.id)}
                      >
                        Изтрий
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div className="users-card">
          <h2 className="profile-section-title">Добави потребител</h2>
          <form onSubmit={handleCreate} className="users-form">
            <div className="users-form-row">
              <label className="login-label">Email</label>
              <input
                className="login-input"
                type="email"
                required
                autoComplete="off"
                value={form.email}
                onChange={e => setField('email', e.target.value)}
              />
            </div>
            <div className="users-form-row">
              <label className="login-label">Парола</label>
              <input
                className="login-input"
                type="password"
                required
                minLength={8}
                autoComplete="new-password"
                value={form.password}
                onChange={e => setField('password', e.target.value)}
              />
            </div>
            <div className="users-form-row">
              <label className="login-label">Роля</label>
              <select
                className="login-input users-select"
                value={form.role}
                onChange={e => setField('role', e.target.value)}
              >
                <option value="user">user</option>
                <option value="admin">admin</option>
              </select>
            </div>
            {formError && <p className="login-error">{formError}</p>}
            <button className="login-btn" type="submit" disabled={formLoading}>
              {formLoading ? 'Добавяне...' : 'Добави'}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}
