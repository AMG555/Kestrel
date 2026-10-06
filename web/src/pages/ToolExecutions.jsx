import { useState, useEffect } from 'react'
import { api } from '../api.js'

export default function ToolExecutionsPage() {
  const [execs, setExecs]       = useState([])
  const [total, setTotal]       = useState(0)
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState('')
  const [filter, setFilter]     = useState({ tool_name: '', status: '' })
  const [tools, setTools]       = useState([])

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
          <div style={{ color: 'var(--muted)', fontSize: 13, marginTop: 2 }}>Complete history of every tool call</div>
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
          <div>No tool executions yet.</div>
        </div>
      ) : (
        <>
          <div style={{ color: 'var(--muted)', fontSize: 12, marginBottom: 12 }}>{total} execution{total !== 1 ? 's' : ''}</div>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ background: 'var(--surface)', borderBottom: '1px solid var(--border)' }}>
                {['Tool', 'Status', 'Duration', 'Output', 'Session', 'Started'].map(h => (
                  <th key={h} style={{ padding: '8px 12px', textAlign: 'left', color: 'var(--muted)', fontWeight: 600 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {execs.map((e, i) => {
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
                    <td style={{ padding: '8px 12px', fontFamily: 'monospace', fontSize: 10, color: 'var(--muted)' }}>
                      {e.session_id ? e.session_id.slice(0, 12) + '…' : '—'}
                    </td>
                    <td style={{ padding: '8px 12px', color: 'var(--muted)' }}>
                      {new Date(e.started_at).toLocaleString()}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </>
      )}
    </div>
  )
}
