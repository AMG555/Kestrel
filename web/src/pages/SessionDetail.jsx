import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../api.js'

const ROLE_COLORS = {
  user: '#3b82d4', assistant: '#22c55e', tool: '#f59e0b', system: '#94a3b8',
}

export default function SessionDetailPage() {
  const { id } = useParams()
  const [session, setSession]   = useState(null)
  const [messages, setMessages] = useState([])
  const [executions, setExecs]  = useState([])
  const [tab, setTab]           = useState('messages') // messages | tools
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState('')

  useEffect(() => {
    setLoading(true)
    Promise.all([
      api.getSession(id),
      api.listToolExecutions({ session_id: id, limit: 100 }),
    ])
      .then(([sd, td]) => {
        setSession(sd.session)
        setMessages(sd.messages || [])
        setExecs(td.executions || [])
        setLoading(false)
      })
      .catch(e => { setError(e.message); setLoading(false) })
  }, [id])

  if (loading) return <div style={{ color: 'var(--muted)', padding: 32 }}>Loading…</div>
  if (error || !session) return (
    <div>
      <Link to="/sessions" style={{ color: 'var(--muted)', fontSize: 13 }}>← Sessions</Link>
      <div style={{ color: 'var(--danger)', marginTop: 16 }}>{error || 'Session not found'}</div>
    </div>
  )

  const statusColor = { active: '#f59e0b', completed: '#22c55e', failed: '#ef4444', cancelled: '#94a3b8' }

  return (
    <div>
      {/* Header */}
      <div style={{ marginBottom: 20 }}>
        <Link to="/sessions" style={{ color: 'var(--muted)', fontSize: 12, textDecoration: 'none' }}>← Sessions</Link>
        <h2 style={{ margin: '6px 0 4px' }}>{session.title}</h2>
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', fontSize: 12, color: 'var(--muted)' }}>
          <span style={{ color: statusColor[session.status] || '#94a3b8', fontWeight: 600 }}>
            ● {session.status}
          </span>
          <span>Mode: {session.agent_mode}</span>
          <span>HITL: {session.hitl_mode}</span>
          <span>Started: {new Date(session.created_at).toLocaleString()}</span>
          {session.project_id && <span>Project: {session.project_id.slice(0, 12)}…</span>}
        </div>
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: 0, borderBottom: '1px solid var(--border)', marginBottom: 16 }}>
        {[['messages', `💬 Messages (${messages.length})`], ['tools', `🔧 Tool Calls (${executions.length})`]].map(([key, label]) => (
          <button key={key} onClick={() => setTab(key)} style={{
            padding: '8px 16px', fontSize: 13, fontWeight: 600, border: 'none', cursor: 'pointer',
            borderBottom: tab === key ? '2px solid var(--accent)' : '2px solid transparent',
            color: tab === key ? 'var(--accent)' : 'var(--muted)', background: 'transparent',
          }}>{label}</button>
        ))}
      </div>

      {tab === 'messages' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {messages.length === 0 && (
            <div style={{ color: 'var(--muted)', fontSize: 13 }}>No messages recorded.</div>
          )}
          {messages.map((m, i) => (
            <div key={i} style={{
              background: 'var(--surface)', borderRadius: 8, padding: '10px 14px',
              borderLeft: `3px solid ${ROLE_COLORS[m.role] || 'var(--border)'}`,
            }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
                <span style={{ fontSize: 11, fontWeight: 700, color: ROLE_COLORS[m.role] || 'var(--muted)', textTransform: 'uppercase' }}>
                  {m.role}
                </span>
                <span style={{ fontSize: 10, color: 'var(--muted)' }}>
                  {new Date(m.created_at).toLocaleTimeString()}
                </span>
              </div>
              <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontSize: 13, margin: 0 }}>
                {m.content}
              </pre>
            </div>
          ))}
        </div>
      )}

      {tab === 'tools' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {executions.length === 0 && (
            <div style={{ color: 'var(--muted)', fontSize: 13 }}>No tool executions in this session.</div>
          )}
          {executions.map((e, i) => {
            const statusColor = { completed: '#22c55e', failed: '#ef4444', running: '#f59e0b', pending: '#94a3b8' }
            const sc = statusColor[e.status] || '#94a3b8'
            return (
              <div key={i} style={{
                background: 'var(--surface)', borderRadius: 8, padding: '10px 14px',
                borderLeft: `3px solid ${sc}`,
              }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
                  <span style={{ fontWeight: 600, fontSize: 13 }}>{e.tool_name}</span>
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    {e.duration_ms > 0 && (
                      <span style={{ fontSize: 11, color: 'var(--muted)' }}>{e.duration_ms}ms</span>
                    )}
                    <span style={{ fontSize: 11, fontWeight: 600, color: sc }}>{e.status}</span>
                  </div>
                </div>
                {e.arguments_json && e.arguments_json !== '{}' && (
                  <details style={{ marginBottom: 6 }}>
                    <summary style={{ fontSize: 11, color: 'var(--muted)', cursor: 'pointer' }}>Arguments</summary>
                    <pre style={{ fontSize: 11, marginTop: 4, overflowX: 'auto' }}>
                      {(() => { try { return JSON.stringify(JSON.parse(e.arguments_json), null, 2) } catch { return e.arguments_json } })()}
                    </pre>
                  </details>
                )}
                {e.result && (
                  <div style={{ background: 'var(--surface2)', borderRadius: 4, padding: '6px 8px', fontSize: 12 }}>
                    <div style={{ color: 'var(--muted)', fontSize: 10, marginBottom: 4 }}>RESULT{e.output_truncated ? ' (truncated)' : ''}</div>
                    <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', margin: 0 }}>{e.result}</pre>
                  </div>
                )}
                {e.error && (
                  <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>Error: {e.error}</div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
