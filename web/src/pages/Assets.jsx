import { useState, useEffect } from 'react'
import { api } from '../api.js'

const TOKEN = () => localStorage.getItem('token') || ''

export default function AssetsPage() {
  const [assets, setAssets] = useState([])
  const [total, setTotal] = useState(0)
  const [search, setSearch] = useState('')
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ host: '', ip: '', port: '', domain: '', protocol: 'https', service: '' })
  const [saving, setSaving] = useState(false)

  const load = () => {
    api.listAssets({ search }).then(d => { setAssets(d.assets || []); setTotal(d.total || 0) })
      .catch(err => setError(err.message))
  }

  useEffect(load, [search])

  const createAsset = async () => {
    setSaving(true)
    try {
      await api.createAsset({ ...form, port: parseInt(form.port) || 0 })
      setShowForm(false)
      setForm({ host: '', ip: '', port: '', domain: '', protocol: 'https', service: '' })
      load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const deleteAsset = async (id) => {
    if (!confirm('Delete this asset?')) return
    await api.deleteAsset(id).catch(err => setError(err.message))
    load()
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-16">
        <h1 style={{ fontSize: 20, fontWeight: 700 }}>Assets <span style={{ color: 'var(--muted)', fontSize: 14, fontWeight: 400 }}>({total})</span></h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <a
            href={`/api/assets/export.csv`}
            onClick={e => { e.preventDefault(); window.location.href = `/api/assets/export.csv` }}
            style={{ textDecoration: 'none' }}
          >
            <button>⬇ Export CSV</button>
          </a>
          <button className="primary" onClick={() => setShowForm(s => !s)}>+ Add Asset</button>
        </div>
      </div>

      {error && <div className="error-msg mb-16">{error}</div>}

      {showForm && (
        <div className="card mb-16">
          <div style={{ fontWeight: 600, marginBottom: 12 }}>New Asset</div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12 }}>
            {[['Host', 'host'], ['IP', 'ip'], ['Port', 'port'], ['Domain', 'domain'], ['Protocol', 'protocol'], ['Service', 'service']].map(([label, key]) => (
              <div key={key}>
                <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>{label}</label>
                <input value={form[key]} onChange={e => setForm(f => ({ ...f, [key]: e.target.value }))} />
              </div>
            ))}
          </div>
          <div className="flex gap-8 mt-16">
            <button className="primary" onClick={createAsset} disabled={saving}>{saving ? 'Saving…' : 'Create'}</button>
            <button onClick={() => setShowForm(false)}>Cancel</button>
          </div>
        </div>
      )}

      <div className="card" style={{ padding: 0 }}>
        <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--border)', display: 'flex', gap: 12 }}>
          <input
            style={{ maxWidth: 260 }}
            placeholder="Search host, IP, domain…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
        </div>
        <table>
          <thead>
            <tr>
              <th>Host / Domain</th>
              <th>IP</th>
              <th>Port</th>
              <th>Protocol</th>
              <th>Service</th>
              <th>Risk</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {(assets || []).length === 0 && (
              <tr><td colSpan={8} style={{ color: 'var(--muted)', textAlign: 'center', padding: 24 }}>No assets found.</td></tr>
            )}
            {(assets || []).map(a => (
              <tr key={a.id}>
                <td>{a.host || a.domain || '—'}</td>
                <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{a.ip || '—'}</td>
                <td>{a.port || '—'}</td>
                <td>{a.protocol || '—'}</td>
                <td>{a.service || '—'}</td>
                <td><span className={`badge badge-${a.risk_level}`}>{a.risk_level}</span></td>
                <td><span className={`badge badge-${a.status}`}>{a.status}</span></td>
                <td>
                  <button style={{ padding: '3px 8px', fontSize: 11 }} onClick={() => deleteAsset(a.id)}>Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
