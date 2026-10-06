import { useState, useEffect } from 'react'
import { api } from '../api.js'

const STATUS_COLORS = {
  pending: '#94a3b8', running: '#f59e0b', completed: '#22c55e',
  failed: '#ef4444', cancelled: '#94a3b8',
}

function StatusBadge({ status }) {
  return (
    <span style={{
      fontSize: 11, padding: '2px 8px', borderRadius: 10, fontWeight: 600,
      background: (STATUS_COLORS[status] || '#94a3b8') + '22',
      color: STATUS_COLORS[status] || '#94a3b8',
    }}>{status}</span>
  )
}

export default function BatchTasksPage() {
  const [queues, setQueues]       = useState([])
  const [selected, setSelected]   = useState(null)
  const [detail, setDetail]       = useState(null)
  const [loading, setLoading]     = useState(true)
  const [error, setError]         = useState('')
  const [showForm, setShowForm]   = useState(false)
  const [form, setForm]           = useState({ title: '', agent_mode: 'single', hitl_mode: 'auto', intents: '' })
  const [saving, setSaving]       = useState(false)

  const loadQueues = () => {
    setLoading(true)
    api.listQueues().then(d => { setQueues(d.queues || []); setLoading(false) })
      .catch(e => { setError(e.message); setLoading(false) })
  }

  const loadDetail = (id) => {
    api.getQueue(id).then(d => setDetail(d)).catch(() => {})
  }

  useEffect(() => { loadQueues() }, [])

  useEffect(() => {
    if (!selected) return
    loadDetail(selected)
    const iv = setInterval(() => loadDetail(selected), 5000)
    return () => clearInterval(iv)
  }, [selected])

  const handleCreate = async (e) => {
    e.preventDefault()
    setSaving(true)
    try {
      await api.createQueue({
        title: form.title,
        agent_mode: form.agent_mode,
        hitl_mode: form.hitl_mode,
        intents: form.intents.split('\n').map(s => s.trim()).filter(Boolean),
      })
      setForm({ title: '', agent_mode: 'single', hitl_mode: 'auto', intents: '' })
      setShowForm(false)
      loadQueues()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const run = async (id) => {
    await api.runQueue(id).catch(e => setError(e.message))
    loadQueues()
    loadDetail(id)
  }

  const cancel = async (id) => {
    await api.cancelQueue(id).catch(e => setError(e.message))
    loadQueues()
  }

  const del = async (id) => {
    if (!confirm('Delete this queue?')) return
    await api.deleteQueue(id).catch(e => setError(e.message))
    if (selected === id) setSelected(null)
    loadQueues()
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h2 style={{ margin: 0 }}>Batch Task Queues</h2>
        <button onClick={() => setShowForm(v => !v)}>{showForm ? 'Cancel' : '+ New Queue'}</button>
      </div>

      {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}

      {showForm && (
        <form onSubmit={handleCreate} style={{
          background: 'var(--surface)', border: '1px solid var(--border)',
          borderRadius: 8, padding: 16, marginBottom: 20, display: 'grid', gap: 10,
        }}>
          <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr', gap: 10 }}>
            <div>
              <label style={{ fontSize: 12, color: 'var(--muted)' }}>Queue Title *</label>
              <input value={form.title} onChange={e => setForm(f => ({ ...f, title: e.target.value }))}
                required style={{ width: '100%', marginTop: 4 }} />
            </div>
            <div>
              <label style={{ fontSize: 12, color: 'var(--muted)' }}>Agent Mode</label>
              <select value={form.agent_mode} onChange={e => setForm(f => ({ ...f, agent_mode: e.target.value }))}
                style={{ width: '100%', marginTop: 4 }}>
                <option value="single">Single</option>
                <option value="plan_execute">Plan-Execute</option>
                <option value="supervisor">Supervisor</option>
              </select>
            </div>
            <div>
              <label style={{ fontSize: 12, color: 'var(--muted)' }}>HITL Mode</label>
              <select value={form.hitl_mode} onChange={e => setForm(f => ({ ...f, hitl_mode: e.target.value }))}
                style={{ width: '100%', marginTop: 4 }}>
                <option value="auto">Auto</option>
                <option value="require_approval">Require Approval</option>
              </select>
            </div>
          </div>
          <div>
            <label style={{ fontSize: 12, color: 'var(--muted)' }}>Intents (one per line)</label>
            <textarea value={form.intents} onChange={e => setForm(f => ({ ...f, intents: e.target.value }))}
              rows={4} style={{ width: '100%', marginTop: 4, fontFamily: 'monospace', fontSize: 12 }}
              placeholder="Enumerate subdomains of example.com&#10;Check open ports on 192.168.1.1" />
          </div>
          <div style={{ textAlign: 'right' }}>
            <button type="submit" disabled={saving} className="primary">{saving ? 'Creating…' : 'Create Queue'}</button>
          </div>
        </form>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: selected ? '320px 1fr' : '1fr', gap: 16 }}>
        {/* Queue list */}
        <div style={{ display: 'grid', gap: 10, alignContent: 'start' }}>
          {loading ? (
            <div style={{ color: 'var(--muted)' }}>Loading…</div>
          ) : queues.length === 0 ? (
            <div style={{ color: 'var(--muted)', textAlign: 'center', padding: 24 }}>No queues yet.</div>
          ) : queues.map(q => (
            <div key={q.id} onClick={() => setSelected(q.id)}
              style={{
                background: 'var(--surface)', border: `1px solid ${selected === q.id ? 'var(--accent)' : 'var(--border)'}`,
                borderRadius: 8, padding: '12px 14px', cursor: 'pointer',
              }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
                <span style={{ fontWeight: 600, fontSize: 13 }}>{q.title}</span>
                <StatusBadge status={q.status} />
              </div>
              <div style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 8 }}>
                Mode: {q.agent_mode} | HITL: {q.hitl_mode}
              </div>
              <div style={{ display: 'flex', gap: 6 }}>
                {q.status === 'pending' && (
                  <button onClick={e => { e.stopPropagation(); run(q.id) }}
                    style={{ fontSize: 11, padding: '3px 10px' }} className="primary">▶ Run</button>
                )}
                {q.status === 'running' && (
                  <button onClick={e => { e.stopPropagation(); cancel(q.id) }}
                    style={{ fontSize: 11, padding: '3px 10px' }}>⏹ Cancel</button>
                )}
                <button onClick={e => { e.stopPropagation(); del(q.id) }}
                  style={{ fontSize: 11, padding: '3px 10px', color: 'var(--danger)' }}>Delete</button>
              </div>
            </div>
          ))}
        </div>

        {/* Detail pane */}
        {selected && detail && (
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 16 }}>
            <div style={{ fontWeight: 600, marginBottom: 12 }}>{detail.queue?.title} — Tasks</div>
            {(detail.tasks || []).map((t, i) => (
              <div key={t.id} style={{
                borderBottom: '1px solid var(--border)', paddingBottom: 10, marginBottom: 10,
              }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                  <span style={{ fontSize: 12, color: 'var(--muted)' }}>#{i + 1}</span>
                  <StatusBadge status={t.status} />
                </div>
                <div style={{ fontSize: 13, marginBottom: 4 }}>{t.intent}</div>
                {t.result && <div style={{ fontSize: 12, color: '#22c55e', whiteSpace: 'pre-wrap' }}>{t.result}</div>}
                {t.error && <div style={{ fontSize: 12, color: 'var(--danger)' }}>{t.error}</div>}
              </div>
            ))}
            {(!detail.tasks || detail.tasks.length === 0) && (
              <div style={{ color: 'var(--muted)', fontSize: 13 }}>No tasks in this queue.</div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
