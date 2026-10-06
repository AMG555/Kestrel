import { useState, useContext } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { AuthCtx } from '../App.jsx'

export default function LoginPage() {
  const { login } = useContext(AuthCtx)
  const navigate = useNavigate()
  const location = useLocation()
  const from = location.state?.from?.pathname || '/'

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const d = await login(username, password)
      if (d.must_change_password) navigate('/change-password', { replace: true })
      else navigate(from, { replace: true })
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
      <div className="card" style={{ width: 360, padding: 32 }}>
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <div style={{ fontSize: 28, fontWeight: 700, color: 'var(--accent)' }}>⚔ Kestrel</div>
          <div style={{ color: 'var(--muted)', fontSize: 12, marginTop: 4 }}>
            AI-native Security Operations Platform<br/>
            <span style={{ color: 'var(--warn)', fontWeight: 600 }}>AUTHORIZED USE ONLY</span>
          </div>
        </div>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: 14 }}>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Username</label>
            <input
              type="text" autoComplete="username" autoFocus
              value={username} onChange={e => setUsername(e.target.value)}
              placeholder="admin"
            />
          </div>
          <div style={{ marginBottom: 20 }}>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Password</label>
            <input
              type="password" autoComplete="current-password"
              value={password} onChange={e => setPassword(e.target.value)}
            />
          </div>

          {error && <div className="error-msg" style={{ marginBottom: 12 }}>{error}</div>}

          <button type="submit" className="primary" disabled={loading} style={{ width: '100%', padding: '10px 0' }}>
            {loading ? 'Signing in…' : 'Sign in'}
          </button>
        </form>

        <div style={{ marginTop: 20, fontSize: 11, color: 'var(--muted)', textAlign: 'center', lineHeight: 1.5 }}>
          By signing in you agree to use this platform only on systems
          you own or are explicitly authorized to test.
        </div>
      </div>
    </div>
  )
}
