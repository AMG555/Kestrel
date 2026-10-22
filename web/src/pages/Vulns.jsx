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

  const exportCSV = () => {
    if (!vulns || vulns.length === 0) {
      setError('No vulnerabilities to export')
      return
    }
    const headers = ['ID', 'Title', 'Severity', 'Status', 'Target', 'Description', 'Recommendation', 'Created At']
    const rows = vulns.map(v => [
      `"${v.id || ''}"`,
      `"${(v.title || '').replace(/"/g, '""')}"`,
      `"${v.severity || ''}"`,
      `"${v.status || ''}"`,
      `"${(v.target || '').replace(/"/g, '""')}"`,
      `"${(v.description || '').replace(/"/g, '""')}"`,
      `"${(v.recommendation || '').replace(/"/g, '""')}"`,
      `"${v.created_at || ''}"`,
    ])
    const csvContent = [headers.join(','), ...rows.map(r => r.join(','))].join('\n')
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `kestrel_vulnerabilities_${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  const exportReport = async () => {
    try {
      const token = localStorage.getItem('kestrel_token') || ''
      const res = await fetch('/api/vulnerabilities/export?mode=summary&group_by=conversation', {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      })
      if (!res.ok) throw new Error(`Export failed (${res.status})`)
      const data = await res.json()
      const content = data.files?.[0]?.content || '# Vulnerability Report\nNo findings.'
      const blob = new Blob([content], { type: 'text/markdown;charset=utf-8;' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `vulnerability_report_${new Date().toISOString().slice(0, 10)}.md`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError('Report export failed: ' + err.message)
    }
  }

  const exportSARIF = async () => {
    try {
      const res = await api.exportSarif(filters)
      if (!res.ok) throw new Error(`SARIF export failed (${res.status})`)
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `kestrel_findings_${new Date().toISOString().slice(0, 10)}.sarif`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError('SARIF export failed: ' + err.message)
    }
  }

  const downloadPoC = async (id, title, format = 'python') => {
    try {
      const res = await api.exportPoC(id, format)
      if (!res.ok) throw new Error(`PoC export failed (${res.status})`)
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      const ext = (format === 'bash' || format === 'sh') ? 'sh' : 'py'
      const cleanTitle = (title || id).toLowerCase().replace(/[^a-z0-9_-]/g, '_').slice(0, 30)
      a.download = `poc_${cleanTitle}.${ext}`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError('PoC download failed: ' + err.message)
    }
  }

  const downloadNuclei = async (id, title) => {
    try {
      const res = await api.exportNuclei(id)
      if (!res.ok) throw new Error(`Nuclei export failed (${res.status})`)
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      const cleanTitle = (title || id).toLowerCase().replace(/[^a-z0-9_-]/g, '_').slice(0, 30)
      a.download = `nuclei_${cleanTitle}.yaml`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError('Nuclei template download failed: ' + err.message)
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-16">
        <h1 style={{ fontSize: 20, fontWeight: 700 }}>Vulnerabilities <span style={{ color: 'var(--muted)', fontSize: 14, fontWeight: 400 }}>({total})</span></h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={exportCSV} title="Export current list to CSV">⬇ CSV</button>
          <button onClick={exportReport} title="Export full Markdown audit report">📜 Report</button>
          <button onClick={exportSARIF} title="Export OASIS SARIF 2.1.0 report for CI/CD">🛡 SARIF</button>
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
            <tr><th>Title</th><th>Severity</th><th>Status</th><th>Target</th><th>Created</th><th style={{ width: 160 }}>Actions</th></tr>
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
                  <div style={{ display: 'flex', gap: 4 }}>
                    <button style={{ padding: '2px 6px', fontSize: 11 }} onClick={() => downloadPoC(v.id, v.title, 'python')} title="Download Standalone Python PoC">🐍 PoC</button>
                    <button style={{ padding: '2px 6px', fontSize: 11 }} onClick={() => downloadNuclei(v.id, v.title)} title="Download Nuclei Regression Template">⚡ Nuclei</button>
                    <button style={{ padding: '2px 6px', fontSize: 11 }} onClick={() => del(v.id)} title="Delete finding">🗑</button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
