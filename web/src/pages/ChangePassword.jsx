import { useState, useContext } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api.js'
import { AuthCtx } from '../App.jsx'

export default function ChangePasswordPage() {
  const { user, setUser } = useContext(AuthCtx)
  const navigate = useNavigate()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    if (next.length < 8) { setError('New password must be at least 8 characters'); return }
    if (next !== confirm) { setError('Passwords do not match'); return }
    setLoading(true)
    try {
      await api.changePassword(current, next)
      setSuccess('Password changed successfully.')
      setUser(u => ({ ...u, must_change_password: false }))
      setTimeout(() => navigate('/'), 1200)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--bg)',
    }}>
      <div className="card" style={{ width: 380, padding: 32 }}>
        <div style={{ marginBottom: 24 }}>
          <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--accent)' }}>Change Password</div>
          <div style={{ color: 'var(--warn)', fontSize: 12, marginTop: 4 }}>
            You must change your password before continuing.
          </div>
        </div>
        <form onSubmit={handleSubmit}>
          {['Current password', 'New password', 'Confirm new password'].map((label, i) => {
            const [val, setter] = [[current, setCurrent], [next, setNext], [confirm, setConfirm]][i]
            return (
              <div key={label} style={{ marginBottom: 14 }}>
                <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>{label}</label>
                <input type="password" value={val} onChange={e => setter(e.target.value)} />
              </div>
            )
          })}
          {error && <div className="error-msg" style={{ marginBottom: 12 }}>{error}</div>}
          {success && <div className="success-msg" style={{ marginBottom: 12 }}>{success}</div>}
          <button type="submit" className="primary" disabled={loading} style={{ width: '100%', padding: '10px 0' }}>
            {loading ? 'Saving…' : 'Change Password'}
          </button>
        </form>
      </div>
    </div>
  )
}
