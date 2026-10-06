import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api.js'

const STATUS_COLORS = { active: '#22c55e', archived: '#94a3b8', completed: '#3b82f6' }

export default function ProjectsPage() {
  const [projects, setProjects] = useState([])
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState('')
  const [showForm, setShowForm] = useState(false)
  const [form, setForm]         = useState({ name: '', description: '', scope: '' })
  const [saving, setSaving]     = useState(false)
  const navigate = useNavigate()

  const load = () => {
    setLoading(true)
    api.listProjects().then(d => { setProjects(d.projects || []); setLoading(false) })
      .catch(e => { setError(e.message); setLoading(false) })
  }

  useEffect(() => { load() }, [])

  const handleCreate = async (e) => {
    e.preventDefault()
    setSaving(true)
    try {
      await api.createProject({
        name: form.name,
        description: form.description,
        scope: form.scope ? form.scope.split(',').map(s => s.trim()).filter(Boolean) : [],
      })
      setForm({ name: '', description: '', scope: '' })
      setShowForm(false)
      load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('Delete this project and all its data?')) return
    await api.deleteProject(id).catch(e => setError(e.message))
    load()
  }

  const badge = (status) => (
    <span style={{
      fontSize: 11, padding: '2px 8px', borderRadius: 10,
      background: (STATUS_COLORS[status] || '#94a3b8') + '22',
      color: STATUS_COLORS[status] || '#94a3b8', fontWeight: 600,
    }}>{status}</span>
  )

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h2 style={{ margin: 0 }}>Projects</h2>
        <button onClick={() => setShowForm(v => !v)}>
          {showForm ? 'Cancel' : '+ New Project'}
        </button>
      </div>

      {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}

      {showForm && (
        <form onSubmit={handleCreate} style={{
          background: 'var(--surface)', border: '1px solid var(--border)',
          borderRadius: 8, padding: 16, marginBottom: 20,
          display: 'grid', gap: 10,
        }}>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
            <div>
              <label style={{ fontSize: 12, color: 'var(--muted)' }}>Name *</label>
              <input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                required style={{ width: '100%', marginTop: 4 }} placeholder="Project name" />
            </div>
            <div>
              <label style={{ fontSize: 12, color: 'var(--muted)' }}>Scope (comma-separated domains/IPs)</label>
              <input value={form.scope} onChange={e => setForm(f => ({ ...f, scope: e.target.value }))}
                style={{ width: '100%', marginTop: 4 }} placeholder="example.com, 10.0.0.0/24" />
            </div>
          </div>
          <div>
            <label style={{ fontSize: 12, color: 'var(--muted)' }}>Description</label>
            <textarea value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
              rows={2} style={{ width: '100%', marginTop: 4 }} placeholder="Optional description" />
          </div>
          <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
            <button type="submit" disabled={saving} className="primary">
              {saving ? 'Creating…' : 'Create Project'}
            </button>
          </div>
        </form>
      )}

      {loading ? (
        <div style={{ color: 'var(--muted)', padding: 24 }}>Loading…</div>
      ) : projects.length === 0 ? (
        <div style={{ color: 'var(--muted)', padding: 24, textAlign: 'center' }}>
          No projects yet. Create one to get started.
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 12 }}>
          {projects.map(p => (
            <div key={p.id} style={{
              background: 'var(--surface)', border: '1px solid var(--border)',
              borderRadius: 8, padding: '14px 16px',
              display: 'flex', alignItems: 'center', gap: 12,
            }}>
              {p.pinned && <span title="Pinned" style={{ fontSize: 14 }}>📌</span>}
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 2 }}>{p.name}</div>
                {p.description && (
                  <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 4 }}>{p.description}</div>
                )}
                {p.scope?.length > 0 && (
                  <div style={{ fontSize: 11, color: 'var(--muted)' }}>
                    Scope: {p.scope.join(', ')}
                  </div>
                )}
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                {badge(p.status || 'active')}
                <button onClick={() => navigate(`/projects/${p.id}/attack-chain`)} style={{ fontSize: 12 }}>
                  Attack Chain
                </button>
                <button onClick={() => handleDelete(p.id)} style={{ fontSize: 12, color: 'var(--danger)' }}>
                  Delete
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
