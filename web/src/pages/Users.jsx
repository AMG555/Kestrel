import { useState, useEffect, useContext } from 'react'
import { api } from '../api.js'
import { AuthCtx } from '../App.jsx'

export default function UsersPage() {
  const { user: me } = useContext(AuthCtx)
  const [users, setUsers] = useState([])
  const [roles, setRoles] = useState([])
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ username: '', password: '', email: '', display_name: '' })
  const [saving, setSaving] = useState(false)

  const load = () => {
    Promise.all([api.listUsers(), api.listRoles()])
      .then(([ud, rd]) => { setUsers(ud.users || []); setRoles(rd.roles || []) })
      .catch(err => setError(err.message))
  }

  useEffect(load, [])

  const createUser = async () => {
    setSaving(true)
    try {
      await api.createUser(form)
      setShowForm(false)
      setForm({ username: '', password: '', email: '', display_name: '' })
      load()
    } catch (err) { setError(err.message) }
    finally { setSaving(false) }
  }

  const deleteUser = async (id) => {
    if (!confirm('Delete this user?')) return
    await api.deleteUser(id).catch(err => setError(err.message))
    load()
  }

  const toggleActive = async (u) => {
    await api.updateUser(u.id, { is_active: !u.is_active }).catch(err => setError(err.message))
    load()
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-16">
        <h1 style={{ fontSize: 20, fontWeight: 700 }}>Users</h1>
        <button className="primary" onClick={() => setShowForm(s => !s)}>+ Add User</button>
      </div>

      {error && <div className="error-msg mb-16">{error}</div>}

      {showForm && (
        <div className="card mb-16">
          <div style={{ fontWeight: 600, marginBottom: 12 }}>New User</div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2,1fr)', gap: 12 }}>
            {[['Username*', 'username'], ['Password*', 'password'], ['Email', 'email'], ['Display Name', 'display_name']].map(([label, key]) => (
              <div key={key}>
                <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>{label}</label>
                <input type={key === 'password' ? 'password' : 'text'} value={form[key]}
                  onChange={e => setForm(f => ({ ...f, [key]: e.target.value }))} />
              </div>
            ))}
          </div>
          <div className="flex gap-8 mt-16">
            <button className="primary" onClick={createUser} disabled={saving || !form.username || !form.password}>{saving ? 'Saving…' : 'Create'}</button>
            <button onClick={() => setShowForm(false)}>Cancel</button>
          </div>
        </div>
      )}

      <div className="card" style={{ padding: 0 }}>
        <table>
          <thead>
            <tr><th>Username</th><th>Display Name</th><th>Email</th><th>Status</th><th>Must Change Pwd</th><th></th></tr>
          </thead>
          <tbody>
            {(users || []).length === 0 && <tr><td colSpan={6} style={{ color: 'var(--muted)', textAlign: 'center', padding: 24 }}>No users.</td></tr>}
            {(users || []).map(u => (
              <tr key={u.id}>
                <td style={{ fontWeight: 600 }}>{u.username}</td>
                <td>{u.display_name || '—'}</td>
                <td style={{ color: 'var(--muted)' }}>{u.email || '—'}</td>
                <td>
                  <span className={`badge badge-${u.is_active ? 'active' : 'info'}`}
                    style={{ cursor: 'pointer' }} onClick={() => toggleActive(u)}>
                    {u.is_active ? 'active' : 'inactive'}
                  </span>
                </td>
                <td style={{ color: u.must_change_password ? 'var(--warn)' : 'var(--muted)', fontSize: 12 }}>
                  {u.must_change_password ? 'Yes' : 'No'}
                </td>
                <td>
                  {u.id !== me?.id && (
                    <button style={{ padding: '3px 8px', fontSize: 11 }} onClick={() => deleteUser(u.id)}>Delete</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
