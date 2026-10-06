import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api.js'

const STATUS_COLORS = {
  active: '#f59e0b', completed: '#22c55e', failed: '#ef4444', cancelled: '#94a3b8',
}

function StatusBadge({ status }) {
  const color = STATUS_COLORS[status] || '#94a3b8'
  return (
    <span style={{
      fontSize: 11, padding: '2px 8px', borderRadius: 10, fontWeight: 600,
      background: color + '22', color,
    }}>{status}</span>
  )
}

function ModeChip({ mode }) {
  const colors = { single: '#3b82d4', plan_execute: '#8b5cf6', supervisor: '#f59e0b' }
  const color = colors[mode] || '#94a3b8'
  return (
    <span style={{
      fontSize: 10, padding: '1px 7px', borderRadius: 8,
      background: color + '22', color, fontWeight: 600, marginLeft: 6,
    }}>{mode}</span>
  )
}

export default function SessionsPage() {
  const [sessions, setSessions] = useState([])
  const [total, setTotal]       = useState(0)
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState('')
  const [filter, setFilter]     = useState({ status: '', search: '' })
  const navigate = useNavigate()

  const load = () => {
    setLoading(true)
    const q = {}
    if (filter.status) q.status = filter.status
    api.listSessions(q)
      .then(d => { setSessions(d.sessions || []); setTotal(d.total || 0); setLoading(false) })
      .catch(e => { setError(e.message); setLoading(false) })
  }

  useEffect(() => { load() }, [filter.status])

  const handlePin = async (id, pinned) => {
    await api.updateSession(id, { pinned: !pinned }).catch(e => setError(e.message))
    load()
  }

  const handleDelete = async (id) => {
    if (!confirm('Delete this session and all its messages?')) return
    await api.deleteSession(id).catch(e => setError(e.message))
    load()
  }

  const fmt = (d) => new Date(d).toLocaleString()

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h2 style={{ margin: 0 }}>Agent Sessions</h2>
          <div style={{ color: 'var(--muted)', fontSize: 13, marginTop: 2 }}>
            All agent runs — click to replay messages and tool calls
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <select value={filter.status} onChange={e => setFilter(f => ({ ...f, status: e.target.value }))}
            style={{ fontSize: 12 }}>
            <option value="">All statuses</option>
            <option value="active">Active</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
            <option value="cancelled">Cancelled</option>
          </select>
          <button onClick={load} style={{ fontSize: 12 }}>↻ Refresh</button>
        </div>
      </div>

      {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}

      {loading ? (
        <div style={{ color: 'var(--muted)', padding: 24 }}>Loading…</div>
      ) : sessions.length === 0 ? (
        <div style={{ color: 'var(--muted)', padding: 32, textAlign: 'center' }}>
          <div style={{ fontSize: 32, marginBottom: 8 }}>🤖</div>
          <div>No sessions yet. Run an agent to get started.</div>
        </div>
      ) : (
        <>
          <div style={{ color: 'var(--muted)', fontSize: 12, marginBottom: 12 }}>{total} session{total !== 1 ? 's' : ''}</div>
          <div style={{ display: 'grid', gap: 8 }}>
            {sessions.map(s => (
              <div key={s.id}
                onClick={() => navigate(`/sessions/${s.id}`)}
                style={{
                  background: 'var(--surface)', border: '1px solid var(--border)',
                  borderRadius: 8, padding: '12px 16px', cursor: 'pointer',
                  display: 'flex', alignItems: 'center', gap: 12,
                  transition: 'border-color .15s',
                }}
                onMouseEnter={e => e.currentTarget.style.borderColor = 'var(--accent)'}
                onMouseLeave={e => e.currentTarget.style.borderColor = 'var(--border)'}
              >
                {s.pinned && <span title="Pinned" style={{ fontSize: 14 }}>📌</span>}
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {s.title || s.id}
                    <ModeChip mode={s.agent_mode} />
                  </div>
                  <div style={{ fontSize: 11, color: 'var(--muted)' }}>
                    {fmt(s.created_at)}
                    {s.project_id && <span style={{ marginLeft: 8 }}>📁 {s.project_id.slice(0, 8)}…</span>}
                    <span style={{ marginLeft: 8 }}>💬 {s.message_count || 0}</span>
                    <span style={{ marginLeft: 8 }}>🔧 {s.tool_exec_count || 0}</span>
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
                  <StatusBadge status={s.status} />
                  <button onClick={e => { e.stopPropagation(); handlePin(s.id, s.pinned) }}
                    title={s.pinned ? 'Unpin' : 'Pin'}
                    style={{ fontSize: 13, background: 'none', border: 'none', cursor: 'pointer', padding: '2px 6px' }}>
                    {s.pinned ? '📌' : '📍'}
                  </button>
                  <button onClick={e => { e.stopPropagation(); handleDelete(s.id) }}
                    style={{ fontSize: 12, color: 'var(--danger)', padding: '2px 8px' }}>
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
