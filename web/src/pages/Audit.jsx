import { useState, useEffect } from 'react'
import { api } from '../api.js'

export default function AuditPage() {
  const [logs, setLogs] = useState([])
  const [total, setTotal] = useState(0)
  const [summary, setSummary] = useState(null)
  const [filters, setFilters] = useState({ category: '', result: '' })
  const [error, setError] = useState('')

  useEffect(() => {
    api.listAudit(filters).then(d => { setLogs(d.logs || []); setTotal(d.total || 0) })
      .catch(err => setError(err.message))
  }, [filters.category, filters.result])

  useEffect(() => {
    fetch('/api/audit/summary', {
      headers: { Authorization: 'Bearer ' + (localStorage.getItem('token') || '') }
    })
      .then(r => r.ok ? r.json() : null)
      .then(d => d && setSummary(d))
      .catch(() => {})
  }, [])

  const resultBadge = (r) => {
    if (r === 'success') return 'badge-active'
    if (r === 'failure' || r === 'blocked') return 'badge-critical'
    return 'badge-info'
  }

  const exportCSV = () => {
    const token = localStorage.getItem('token') || ''
    fetch('/api/audit/export.csv', { headers: { Authorization: 'Bearer ' + token } })
      .then(r => r.blob())
      .then(blob => {
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = 'audit_export.csv'
        a.click()
        URL.revokeObjectURL(url)
      })
      .catch(() => setError('Export failed'))
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-16">
        <h1 style={{ fontSize: 20, fontWeight: 700 }}>Audit Logs <span style={{ color: 'var(--muted)', fontSize: 14, fontWeight: 400 }}>({total})</span></h1>
        <button onClick={exportCSV}>⬇ Export CSV</button>
      </div>

      {/* Summary stats */}
      {summary && (
        <div style={{ display: 'flex', gap: 12, marginBottom: 16 }}>
          {[
            { label: 'Total Events', value: summary.total, color: 'var(--accent)' },
            { label: 'Failures', value: summary.failures, color: '#ef4444' },
            { label: 'Last 7 Days', value: summary.recent_7d, color: '#f59e0b' },
          ].map(s => (
            <div key={s.label} className="card" style={{ flex: 1, textAlign: 'center', padding: '12px 16px' }}>
              <div style={{ fontSize: 22, fontWeight: 700, color: s.color }}>{s.value}</div>
              <div style={{ fontSize: 12, color: 'var(--muted)', marginTop: 2 }}>{s.label}</div>
            </div>
          ))}
        </div>
      )}

      {error && <div className="error-msg mb-16">{error}</div>}

      <div className="card" style={{ padding: 0 }}>
        <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--border)', display: 'flex', gap: 12 }}>
          <input style={{ maxWidth: 180 }} placeholder="Category" value={filters.category}
            onChange={e => setFilters(f => ({ ...f, category: e.target.value }))} />
          <select style={{ width: 140 }} value={filters.result} onChange={e => setFilters(f => ({ ...f, result: e.target.value }))}>
            <option value="">All results</option>
            <option value="success">Success</option>
            <option value="failure">Failure</option>
            <option value="blocked">Blocked</option>
          </select>
        </div>
        <table>
          <thead>
            <tr><th>Timestamp</th><th>Actor</th><th>Category</th><th>Action</th><th>Resource</th><th>Result</th><th>Message</th></tr>
          </thead>
          <tbody>
            {(logs || []).length === 0 && <tr><td colSpan={7} style={{ color: 'var(--muted)', textAlign: 'center', padding: 24 }}>No audit records found.</td></tr>}
            {(logs || []).map(l => (
              <tr key={l.id}>
                <td style={{ color: 'var(--muted)', fontSize: 11, whiteSpace: 'nowrap' }}>{new Date(l.created_at).toISOString().replace('T', ' ').slice(0, 19)}</td>
                <td style={{ fontSize: 12 }}>{l.actor_name || l.actor_id || '—'}</td>
                <td style={{ fontSize: 12 }}>{l.category}</td>
                <td style={{ fontSize: 12, fontFamily: 'monospace' }}>{l.action}</td>
                <td style={{ fontSize: 11, color: 'var(--muted)', fontFamily: 'monospace' }}>{l.resource_type}{l.resource_id ? ' / ' + l.resource_id.slice(0, 8) + '…' : ''}</td>
                <td><span className={`badge ${resultBadge(l.result)}`}>{l.result}</span></td>
                <td style={{ color: 'var(--muted)', fontSize: 12, maxWidth: 280, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{l.message}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
