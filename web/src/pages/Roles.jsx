import { useState, useEffect } from 'react'
import { api } from '../api.js'

const ALL_TOOLS = ['subdomain_enum', 'http_probe', 'dns_lookup']
const ALL_PERMS = ['read:assets', 'write:assets', 'read:vulns', 'write:vulns', 'run:agent', 'use:tools']

export default function RolesPage() {
  const [roles, setRoles] = useState([])
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '', description: '', permissions: [], allowed_tools: [], hitl_mode: 'auto' })
  const [saving, setSaving] = useState(false)

  const load = () => {
    api.listRoles().then(d => setRoles(d.roles || [])).catch(err => setError(err.message))
  }

  useEffect(load, [])

  const toggleItem = (list, item) =>
    list.includes(item) ? list.filter(x => x !== item) : [...list, item]

  const create = async () => {
    setSaving(true)
    try {
      await api.createRole(form)
      setShowForm(false)
      setForm({ name: '', description: '', permissions: [], allowed_tools: [], hitl_mode: 'auto' })
      load()
    } catch (err) { setError(err.message) }
    finally { setSaving(false) }
  }

  const deleteRole = async (id) => {
    if (!confirm('Delete this role?')) return
    await api.deleteRole(id).catch(err => setError(err.message))
    load()
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-16">
        <h1 style={{ fontSize: 20, fontWeight: 700 }}>Roles</h1>
        <button className="primary" onClick={() => setShowForm(s => !s)}>+ Add Role</button>
      </div>

      {error && <div className="error-msg mb-16">{error}</div>}

      {showForm && (
        <div className="card mb-16">
          <div style={{ fontWeight: 600, marginBottom: 12 }}>New Role</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginBottom: 12 }}>
            <div>
              <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Name*</label>
              <input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} />
            </div>
            <div>
              <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>HITL Mode</label>
              <select value={form.hitl_mode} onChange={e => setForm(f => ({ ...f, hitl_mode: e.target.value }))}>
                <option value="auto">Auto</option>
                <option value="require_approval">Require Approval</option>
              </select>
            </div>
            <div style={{ gridColumn: '1/-1' }}>
              <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Description</label>
              <input value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} />
            </div>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 12 }}>
            <div>
              <div style={{ color: 'var(--muted)', fontSize: 12, marginBottom: 6 }}>Permissions</div>
              {ALL_PERMS.map(p => (
                <label key={p} style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4, fontSize: 13, cursor: 'pointer' }}>
                  <input type="checkbox" checked={form.permissions.includes(p)}
                    onChange={() => setForm(f => ({ ...f, permissions: toggleItem(f.permissions, p) }))} />
                  {p}
                </label>
              ))}
            </div>
            <div>
              <div style={{ color: 'var(--muted)', fontSize: 12, marginBottom: 6 }}>Allowed Tools</div>
              {ALL_TOOLS.map(t => (
                <label key={t} style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4, fontSize: 13, cursor: 'pointer' }}>
                  <input type="checkbox" checked={form.allowed_tools.includes(t)}
                    onChange={() => setForm(f => ({ ...f, allowed_tools: toggleItem(f.allowed_tools, t) }))} />
                  {t}
                </label>
              ))}
            </div>
          </div>
          <div className="flex gap-8">
            <button className="primary" onClick={create} disabled={saving || !form.name}>{saving ? 'Saving…' : 'Create'}</button>
            <button onClick={() => setShowForm(false)}>Cancel</button>
          </div>
        </div>
      )}

      <div className="card" style={{ padding: 0 }}>
        <table>
          <thead>
            <tr><th>Name</th><th>Description</th><th>HITL Mode</th><th>Permissions</th><th>Allowed Tools</th><th>Type</th><th></th></tr>
          </thead>
          <tbody>
            {(roles || []).length === 0 && <tr><td colSpan={7} style={{ color: 'var(--muted)', textAlign: 'center', padding: 24 }}>No roles.</td></tr>}
            {(roles || []).map(r => (
              <tr key={r.id}>
                <td style={{ fontWeight: 600 }}>{r.name}</td>
                <td style={{ color: 'var(--muted)', fontSize: 12 }}>{r.description || '—'}</td>
                <td><span className="badge badge-medium">{r.hitl_mode}</span></td>
                <td style={{ fontSize: 11 }}>{(r.permissions || []).join(', ') || '—'}</td>
                <td style={{ fontSize: 11 }}>{(r.allowed_tools || []).join(', ') || 'none'}</td>
                <td>
                  <span className={`badge ${r.is_system ? 'badge-info' : 'badge-medium'}`}>
                    {r.is_system ? 'system' : 'custom'}
                  </span>
                </td>
                <td>
                  {!r.is_system && (
                    <button style={{ padding: '3px 8px', fontSize: 11 }} onClick={() => deleteRole(r.id)}>Delete</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
