import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api.js'

export default function ToolExecutionsPage() {
  const [execs, setExecs]       = useState([])
  const [total, setTotal]       = useState(0)
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState('')
  const [filter, setFilter]     = useState({ tool_name: '', status: '' })
  const [tools, setTools]       = useState([])
  const [inspectItem, setInspectItem] = useState(null)

  const load = () => {
    setLoading(true)
    const q = { limit: 100 }
    if (filter.tool_name) q.tool_name = filter.tool_name
    if (filter.status) q.status = filter.status
    api.listToolExecutions(q)
      .then(d => { setExecs(d.executions || []); setTotal(d.total || 0); setLoading(false) })
      .catch(e => { setError(e.message); setLoading(false) })
  }

  useEffect(() => {
    api.listTools().then(d => setTools(d.tools || []))
    load()
  }, [])

  useEffect(() => { load() }, [filter])

  const statusColor = { completed: '#22c55e', failed: '#ef4444', running: '#f59e0b', pending: '#94a3b8', cancelled: '#94a3b8' }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h2 style={{ margin: 0 }}>Tool Executions</h2>
          <div style={{ color: 'var(--muted)', fontSize: 13, marginTop: 2 }}>Complete audit trail of every security tool invocation</div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <select value={filter.tool_name} onChange={e => setFilter(f => ({ ...f, tool_name: e.target.value }))}
            style={{ fontSize: 12 }}>
            <option value="">All tools</option>
            {tools.map(t => <option key={t.name} value={t.name}>{t.name}</option>)}
          </select>
          <select value={filter.status} onChange={e => setFilter(f => ({ ...f, status: e.target.value }))}
            style={{ fontSize: 12 }}>
            <option value="">All statuses</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
            <option value="running">Running</option>
            <option value="pending">Pending</option>
          </select>
          <button onClick={load} style={{ fontSize: 12 }}>↻ Refresh</button>
        </div>
      </div>

      {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}

      {loading ? (
        <div style={{ color: 'var(--muted)', padding: 24 }}>Loading…</div>
      ) : execs.length === 0 ? (
        <div style={{ color: 'var(--muted)', padding: 32, textAlign: 'center' }}>
          <div style={{ fontSize: 32, marginBottom: 8 }}>🔧</div>
          <div>No tool executions recorded yet.</div>
        </div>
      ) : (
        <>
          <div style={{ color: 'var(--muted)', fontSize: 12, marginBottom: 12 }}>{total} execution{total !== 1 ? 's' : ''}</div>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ background: 'var(--surface)', borderBottom: '1px solid var(--border)' }}>
                {['Tool', 'Status', 'Duration', 'Output', 'Session', 'Started', 'Action'].map(h => (
                  <th key={h} style={{ padding: '8px 12px', textAlign: 'left', color: 'var(--muted)', fontWeight: 600 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {execs.map((e) => {
                const sc = statusColor[e.status] || '#94a3b8'
                return (
                  <tr key={e.id} style={{ borderBottom: '1px solid var(--border)' }}>
                    <td style={{ padding: '8px 12px', fontWeight: 600 }}>{e.tool_name}</td>
                    <td style={{ padding: '8px 12px' }}>
                      <span style={{ color: sc, fontWeight: 600 }}>{e.status}</span>
                    </td>
                    <td style={{ padding: '8px 12px', color: 'var(--muted)' }}>
                      {e.duration_ms > 0 ? `${e.duration_ms}ms` : '—'}
                    </td>
                    <td style={{ padding: '8px 12px', color: 'var(--muted)' }}>
                      {e.output_bytes > 0 ? `${(e.output_bytes / 1024).toFixed(1)} KB` : '—'}
                      {e.output_truncated && <span style={{ color: 'var(--warn)', marginLeft: 4 }} title="Truncated">✂</span>}
                    </td>
                    <td style={{ padding: '8px 12px', fontSize: 11 }}>
                      {e.session_id ? (
                        <Link to={`/sessions/${e.session_id}`} style={{ fontFamily: 'monospace' }}>
                          {e.session_id.slice(0, 10)}…
                        </Link>
                      ) : '—'}
                    </td>
                    <td style={{ padding: '8px 12px', color: 'var(--muted)' }}>
                      {new Date(e.started_at).toLocaleString()}
                    </td>
                    <td style={{ padding: '8px 12px' }}>
                      <button
                        onClick={() => setInspectItem(e)}
                        style={{ fontSize: 11, padding: '2px 8px' }}
                      >
                        Inspect
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </>
      )}

      {/* Execution Detail Modal */}
      {inspectItem && (
        <div style={{
          position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
          background: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
          padding: 24,
        }}>
          <div style={{
            background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8,
            maxWidth: 720, width: '100%', maxHeight: '85vh', display: 'flex', flexDirection: 'column', overflow: 'hidden',
          }}>
            <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div style={{ fontWeight: 600, fontSize: 15 }}>
                {inspectItem.tool_name} — Execution Details
              </div>
              <button onClick={() => setInspectItem(null)} style={{ background: 'none', border: 'none', fontSize: 16 }}>✕</button>
            </div>
            <div style={{ padding: 18, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 14 }}>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10, fontSize: 12 }}>
                <div><span style={{ color: 'var(--muted)' }}>Status:</span> <strong>{inspectItem.status}</strong></div>
                <div><span style={{ color: 'var(--muted)' }}>Duration:</span> <strong>{inspectItem.duration_ms || 0}ms</strong></div>
                <div><span style={{ color: 'var(--muted)' }}>Started:</span> <strong>{new Date(inspectItem.started_at).toLocaleTimeString()}</strong></div>
              </div>

              {inspectItem.arguments_json && inspectItem.arguments_json !== '{}' && (
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 4 }}>Arguments JSON:</div>
                  <pre style={{ background: 'var(--surface2)', padding: 10, borderRadius: 6, fontSize: 11, overflowX: 'auto', margin: 0 }}>
                    {(() => { try { return JSON.stringify(JSON.parse(inspectItem.arguments_json), null, 2) } catch { return inspectItem.arguments_json } })()}
                  </pre>
                </div>
              )}

              {inspectItem.result && (
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 4 }}>Execution Output:</div>
                  <pre style={{ background: 'var(--surface2)', padding: 10, borderRadius: 6, fontSize: 11, maxHeight: 240, overflowY: 'auto', whiteSpace: 'pre-wrap', margin: 0 }}>
                    {inspectItem.result}
                  </pre>
                </div>
              )}

              {inspectItem.error && (
                <div style={{ color: 'var(--danger)', fontSize: 12 }}>
                  <strong>Error:</strong> {inspectItem.error}
                </div>
              )}
            </div>
            <div style={{ padding: '12px 18px', borderTop: '1px solid var(--border)', textAlign: 'right' }}>
              <button onClick={() => setInspectItem(null)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
