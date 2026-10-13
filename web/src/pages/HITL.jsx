import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
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
      await api.decideHITL(id, decision)
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
        <div>
          <h2 style={{ margin: 0 }}>Human-in-the-Loop Approvals</h2>
          <p style={{ color: 'var(--muted)', fontSize: 13, margin: '4px 0 0' }}>
            Security tool calls awaiting operator sign-off before host execution.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <span style={{ fontSize: 11, color: '#22c55e', background: '#22c55e15', padding: '3px 8px', borderRadius: 12, border: '1px solid #22c55e44' }}>
            ● Polling live (8s)
          </span>
          <button onClick={load} style={{ fontSize: 12 }}>↻ Refresh</button>
        </div>
      </div>

      {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}

      {loading && items.length === 0 ? (
        <div style={{ color: 'var(--muted)', padding: 24 }}>Loading approvals…</div>
      ) : items.length === 0 ? (
        <div style={{ color: 'var(--muted)', padding: 32, textAlign: 'center', background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8 }}>
          <div style={{ fontSize: 28, marginBottom: 8 }}>✓</div>
          <div>No pending approval requests. All agent tools are authorized.</div>
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 12 }}>
          {items.map(item => (
            <div key={item.id} style={{
              background: 'var(--surface)', border: '1px solid var(--border)',
              borderRadius: 8, padding: 16,
            }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--warn)' }}>⚠ {item.tool_name}</span>
                <span style={{ fontSize: 11, color: 'var(--muted)' }}>{timeAgo(item.created_at)}</span>
              </div>
              <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 8 }}>
                Session:{' '}
                {item.session_id ? (
                  <Link to={`/sessions/${item.session_id}`} style={{ fontSize: 11, fontWeight: 600 }}>
                    {item.session_id} →
                  </Link>
                ) : (
                  <span>—</span>
                )}
                {item.user_id && <span style={{ marginLeft: 8 }}>| User: <code style={{ fontSize: 11 }}>{item.user_id}</code></span>}
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
                  <summary style={{ fontSize: 12, color: 'var(--muted)', cursor: 'pointer' }}>Arguments schema</summary>
                  <pre style={{ fontSize: 11, marginTop: 6, overflowX: 'auto', background: 'var(--surface2)', padding: 8, borderRadius: 4 }}>
                    {JSON.stringify(item.arguments, null, 2)}
                  </pre>
                </details>
              )}
              <div style={{ display: 'flex', gap: 8 }}>
                <button
                  onClick={() => decide(item.id, 'approve')}
                  disabled={deciding !== null}
                  style={{ background: '#22c55e22', color: '#22c55e', border: '1px solid #22c55e55', fontWeight: 600 }}
                >
                  {deciding === item.id + 'approve' ? 'Approving…' : '✓ Authorize Tool'}
                </button>
                <button
                  onClick={() => decide(item.id, 'reject')}
                  disabled={deciding !== null}
                  style={{ background: '#ef444422', color: '#ef4444', border: '1px solid #ef444455' }}
                >
                  {deciding === item.id + 'reject' ? 'Rejecting…' : '✗ Reject Execution'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
