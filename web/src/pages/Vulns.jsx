import { useState, useEffect } from 'react'
import { api } from '../api.js'

const SEVERITIES = ['critical', 'high', 'medium', 'low', 'info']
const STATUSES = ['open', 'in_progress', 'resolved', 'accepted_risk', 'false_positive']

export default function VulnsPage() {
  const [vulns, setVulns] = useState([])
  const [total, setTotal] = useState(0)
  const [filters, setFilters] = useState({ severity: '', status: '' })
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ title: '', severity: 'medium', description: '', target: '', recommendation: '' })
  const [saving, setSaving] = useState(false)
  const [editing, setEditing] = useState(null)

  const load = () => {
    api.listVulns(filters).then(d => { setVulns(d.vulnerabilities || []); setTotal(d.total || 0) })
      .catch(err => setError(err.message))
  }

  useEffect(load, [filters.severity, filters.status])

  const create = async () => {
    setSaving(true)
    try {
      await api.createVuln(form)
      setShowForm(false)
      setForm({ title: '', severity: 'medium', description: '', target: '', recommendation: '' })
      load()
    } catch (err) { setError(err.message) }
    finally { setSaving(false) }
  }

  const update = async (id, data) => {
    await api.updateVuln(id, data).catch(err => setError(err.message))
    setEditing(null)
    load()
  }

  const del = async (id) => {
    if (!confirm('Delete vulnerability?')) return
    await api.deleteVuln(id).catch(err => setError(err.message))
    load()
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-16">
        <h1 style={{ fontSize: 20, fontWeight: 700 }}>Vulnerabilities <span style={{ color: 'var(--muted)', fontSize: 14, fontWeight: 400 }}>({total})</span></h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={() => { window.location.href = '/api/vulnerabilities/export.csv' }}>⬇ Export CSV</button>
          <button className="primary" onClick={() => setShowForm(s => !s)}>+ Add</button>
        </div>
      </div>

      {error && <div className="error-msg mb-16">{error}</div>}

      {showForm && (
        <div className="card mb-16">
          <div style={{ fontWeight: 600, marginBottom: 12 }}>New Vulnerability</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <div style={{ gridColumn: '1/-1' }}>
              <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Title</label>
              <input value={form.title} onChange={e => setForm(f => ({ ...f, title: e.target.value }))} />
            </div>
            <div>
              <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Severity</label>
              <select value={form.severity} onChange={e => setForm(f => ({ ...f, severity: e.target.value }))}>
                {SEVERITIES.map(s => <option key={s} value={s}>{s}</option>)}
              </select>
            </div>
            <div>
              <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Target</label>
              <input value={form.target} onChange={e => setForm(f => ({ ...f, target: e.target.value }))} />
            </div>
            <div style={{ gridColumn: '1/-1' }}>
              <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Description</label>
              <textarea rows={3} value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} />
            </div>
          </div>
          <div className="flex gap-8 mt-16">
            <button className="primary" onClick={create} disabled={saving || !form.title}>{saving ? 'Saving…' : 'Create'}</button>
            <button onClick={() => setShowForm(false)}>Cancel</button>
          </div>
        </div>
      )}

      <div className="card" style={{ padding: 0 }}>
        <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--border)', display: 'flex', gap: 12 }}>
          <select style={{ width: 140 }} value={filters.severity} onChange={e => setFilters(f => ({ ...f, severity: e.target.value }))}>
            <option value="">All severities</option>
            {SEVERITIES.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
          <select style={{ width: 140 }} value={filters.status} onChange={e => setFilters(f => ({ ...f, status: e.target.value }))}>
            <option value="">All statuses</option>
            {STATUSES.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
        </div>
        <table>
          <thead>
            <tr><th>Title</th><th>Severity</th><th>Status</th><th>Target</th><th>Created</th><th></th></tr>
          </thead>
          <tbody>
            {(vulns || []).length === 0 && <tr><td colSpan={6} style={{ color: 'var(--muted)', textAlign: 'center', padding: 24 }}>No vulnerabilities found.</td></tr>}
            {(vulns || []).map(v => (
              <tr key={v.id}>
                <td>{v.title}</td>
                <td><span className={`badge badge-${v.severity}`}>{v.severity}</span></td>
                <td>
                  <select
                    value={v.status}
                    style={{ width: 120, padding: '2px 6px', fontSize: 12 }}
                    onChange={e => update(v.id, { severity: v.severity, status: e.target.value, description: v.description, recommendation: v.recommendation })}
                  >
                    {STATUSES.map(s => <option key={s} value={s}>{s}</option>)}
                  </select>
                </td>
                <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{v.target || '—'}</td>
                <td style={{ color: 'var(--muted)', fontSize: 12 }}>{new Date(v.created_at).toLocaleDateString()}</td>
                <td>
                  <button style={{ padding: '3px 8px', fontSize: 11 }} onClick={() => del(v.id)}>Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
