import { useState, useEffect } from 'react'
import { api } from '../api.js'

export default function ConversationsPage() {
  const [conversations, setConversations] = useState([])
  const [selected, setSelected]           = useState(null)
  const [messages, setMessages]           = useState([])
  const [loading, setLoading]             = useState(true)
  const [loadingMsgs, setLoadingMsgs]     = useState(false)
  const [error, setError]                 = useState('')

  const load = () => {
    setLoading(true)
    api.listConversations()
      .then(d => { setConversations(d.conversations || []); setLoading(false) })
      .catch(e => { setError(e.message); setLoading(false) })
  }

  useEffect(() => { load() }, [])

  const selectConv = async (conv) => {
    setSelected(conv)
    setMessages([])
    setLoadingMsgs(true)
    api.getConvMessages(conv.id)
      .then(d => { setMessages(d.messages || []); setLoadingMsgs(false) })
      .catch(() => setLoadingMsgs(false))
  }

  const handleDelete = async (id) => {
    if (!confirm('Delete this conversation?')) return
    await api.deleteConversation(id).catch(e => setError(e.message))
    if (selected?.id === id) setSelected(null)
    load()
  }

  const roleColor = { user: 'var(--accent)', assistant: '#22c55e', system: '#94a3b8', tool: '#f59e0b' }

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: 16, height: 'calc(100vh - 80px)' }}>
      {/* Sidebar list */}
      <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)', fontWeight: 600, fontSize: 14 }}>
          Conversations ({conversations.length})
        </div>
        {error && <div style={{ padding: 10, color: 'var(--danger)', fontSize: 12 }}>{error}</div>}
        {loading ? (
          <div style={{ padding: 16, color: 'var(--muted)', fontSize: 13 }}>Loading…</div>
        ) : conversations.length === 0 ? (
          <div style={{ padding: 16, color: 'var(--muted)', fontSize: 13 }}>No conversations yet.</div>
        ) : (
          <div style={{ flex: 1, overflowY: 'auto' }}>
            {conversations.map(c => (
              <div
                key={c.id}
                onClick={() => selectConv(c)}
                style={{
                  padding: '10px 14px', cursor: 'pointer', borderBottom: '1px solid var(--border)',
                  background: selected?.id === c.id ? 'var(--surface2)' : 'transparent',
                  borderLeft: selected?.id === c.id ? '3px solid var(--accent)' : '3px solid transparent',
                }}
              >
                <div style={{ fontWeight: 500, fontSize: 13, marginBottom: 2 }}>{c.title || 'Untitled'}</div>
                <div style={{ fontSize: 11, color: 'var(--muted)' }}>{c.agent_mode || 'single'}</div>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 4 }}>
                  <span style={{ fontSize: 10, color: 'var(--muted)' }}>
                    {new Date(c.created_at).toLocaleDateString()}
                  </span>
                  <button onClick={(e) => { e.stopPropagation(); handleDelete(c.id) }}
                    style={{ fontSize: 10, padding: '1px 6px', color: 'var(--danger)' }}>
                    ✕
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Message view */}
      <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {!selected ? (
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--muted)', fontSize: 14 }}>
            Select a conversation to view its messages.
          </div>
        ) : (
          <>
            <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--border)' }}>
              <div style={{ fontWeight: 600, fontSize: 14 }}>{selected.title}</div>
              <div style={{ fontSize: 11, color: 'var(--muted)' }}>Mode: {selected.agent_mode || 'single'}</div>
            </div>
            <div style={{ flex: 1, overflowY: 'auto', padding: '12px 16px', display: 'flex', flexDirection: 'column', gap: 10 }}>
              {loadingMsgs ? (
                <div style={{ color: 'var(--muted)', fontSize: 13 }}>Loading messages…</div>
              ) : messages.length === 0 ? (
                <div style={{ color: 'var(--muted)', fontSize: 13 }}>No messages in this conversation.</div>
              ) : messages.map((m, i) => (
                <div key={i} style={{
                  background: 'var(--surface2)', borderRadius: 8, padding: '8px 12px',
                  borderLeft: `3px solid ${roleColor[m.role] || 'var(--border)'}`,
                }}>
                  <div style={{ fontSize: 11, color: roleColor[m.role] || 'var(--muted)', fontWeight: 600, marginBottom: 4 }}>
                    {m.role.toUpperCase()}
                  </div>
                  <div style={{ fontSize: 13, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                    {m.content}
                  </div>
                  <div style={{ fontSize: 10, color: 'var(--muted)', marginTop: 4 }}>
                    {new Date(m.created_at).toLocaleString()}
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
