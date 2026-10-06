import { useState, useEffect } from 'react'
import { api } from '../api.js'

function timeAgo(dateStr) {
  const ms = Date.now() - new Date(dateStr).getTime()
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  return `${Math.floor(m / 60)}h ago`
}

export default function HITLPage() {
  const [items, setItems]     = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState('')
  const [deciding, setDeciding] = useState(null)

  const load = () => {
    setLoading(true)
    api.listPendingHITL()
      .then(d => { setItems(d.items || []); setLoading(false) })
      .catch(e => { setError(e.message); setLoading(false) })
  }

  useEffect(() => {
    load()
    const iv = setInterval(load, 8000)
    return () => clearInterval(iv)
  }, [])

  const decide = async (id, decision) => {
    setDeciding(id + decision)
    try {
      await api.decideHITL(id, { decision })
      load()
    } catch (err) {
      setError(err.message)
    } finally {
      setDeciding(null)
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h2 style={{ margin: 0 }}>Human-in-the-Loop Approvals</h2>
        <button onClick={load} style={{ fontSize: 12 }}>↻ Refresh</button>
      </div>
      <p style={{ color: 'var(--muted)', fontSize: 13, marginTop: -12, marginBottom: 20 }}>
        Tool calls awaiting explicit operator sign-off before execution.
      </p>

      {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}

      {loading ? (
        <div style={{ color: 'var(--muted)', padding: 24 }}>Loading…</div>
      ) : items.length === 0 ? (
        <div style={{ color: 'var(--muted)', padding: 24, textAlign: 'center' }}>
          No pending approval requests.
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 12 }}>
          {items.map(item => (
            <div key={item.id} style={{
              background: 'var(--surface)', border: '1px solid var(--border)',
              borderRadius: 8, padding: 16,
            }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                <span style={{ fontWeight: 600, fontSize: 14 }}>{item.tool_name}</span>
                <span style={{ fontSize: 11, color: 'var(--muted)' }}>{timeAgo(item.created_at)}</span>
              </div>
              <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 8 }}>
                Session: <code style={{ fontSize: 11 }}>{item.session_id}</code>
                {' | '}User: <code style={{ fontSize: 11 }}>{item.user_id}</code>
              </div>
              {item.context_summary && (
                <div style={{
                  fontSize: 12, background: 'var(--surface2)', borderRadius: 6,
                  padding: '8px 10px', marginBottom: 12, whiteSpace: 'pre-wrap',
                }}>
                  {item.context_summary}
                </div>
              )}
              {item.arguments && (
                <details style={{ marginBottom: 12 }}>
                  <summary style={{ fontSize: 12, color: 'var(--muted)', cursor: 'pointer' }}>Arguments</summary>
                  <pre style={{ fontSize: 11, marginTop: 6, overflowX: 'auto' }}>
                    {JSON.stringify(item.arguments, null, 2)}
                  </pre>
                </details>
              )}
              <div style={{ display: 'flex', gap: 8 }}>
                <button
                  onClick={() => decide(item.id, 'approved')}
                  disabled={deciding !== null}
                  style={{ background: '#22c55e22', color: '#22c55e', border: '1px solid #22c55e55', fontWeight: 600 }}
                >
                  {deciding === item.id + 'approved' ? 'Approving…' : '✓ Approve'}
                </button>
                <button
                  onClick={() => decide(item.id, 'rejected')}
                  disabled={deciding !== null}
                  style={{ background: '#ef444422', color: '#ef4444', border: '1px solid #ef444455' }}
                >
                  {deciding === item.id + 'rejected' ? 'Rejecting…' : '✗ Reject'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
