import { useState, useEffect } from 'react'
import { api } from '../api.js'

function StatCard({ label, value, color }) {
  return (
    <div className="card" style={{ textAlign: 'center', padding: '20px 16px' }}>
      <div style={{ fontSize: 32, fontWeight: 700, color: color || 'var(--accent)' }}>{value ?? '—'}</div>
      <div style={{ color: 'var(--muted)', fontSize: 12, marginTop: 4 }}>{label}</div>
    </div>
  )
}

export default function Dashboard() {
  const [stats, setStats] = useState(null)
  const [info, setInfo] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([api.stats(), api.systemInfo()])
      .then(([s, i]) => { setStats(s); setInfo(i) })
      .catch(err => setError(err.message))
  }, [])

  return (
    <div>
      <div className="flex items-center justify-between mb-16">
        <h1 style={{ fontSize: 20, fontWeight: 700 }}>Dashboard</h1>
        <span style={{
          fontSize: 11, padding: '3px 10px', borderRadius: 12,
          background: '#d2992222', color: 'var(--warn)', fontWeight: 600,
        }}>
          ⚠ Under Development
        </span>
      </div>

      {error && <div className="error-msg mb-16">{error}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(160px,1fr))', gap: 16, marginBottom: 28 }}>
        <StatCard label="Tools Available" value={stats?.tools_available} color="var(--accent)" />
        <StatCard label="Tool Executions" value={stats?.tool_executions} />
        <StatCard label="Assets" value={stats?.assets} color="var(--success)" />
        <StatCard label="Vulnerabilities" value={stats?.vulnerabilities} color="var(--danger)" />
        <StatCard label="Agent Sessions" value={stats?.agent_sessions} />
      </div>

      {info && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
          <div className="card">
            <div style={{ fontWeight: 600, marginBottom: 12 }}>Platform Status</div>
            <table>
              <tbody>
                <tr><td style={{ color: 'var(--muted)', width: 140 }}>Version</td><td>{info.version}</td></tr>
                <tr><td style={{ color: 'var(--muted)' }}>Status</td>
                  <td><span className="badge badge-medium">{info.status}</span></td></tr>
                <tr><td style={{ color: 'var(--muted)' }}>Agent Modes</td><td>{info.agent_modes?.join(', ')}</td></tr>
                <tr><td style={{ color: 'var(--muted)' }}>HITL Modes</td><td>{info.hitl_modes?.join(', ')}</td></tr>
              </tbody>
            </table>
          </div>
          <div className="card">
            <div style={{ fontWeight: 600, marginBottom: 12 }}>Available Tools</div>
            {info.tools?.map(t => (
              <div key={t} style={{
                display: 'inline-block', margin: '0 6px 6px 0', padding: '3px 10px',
                background: 'var(--surface2)', borderRadius: 12, fontSize: 12,
                border: '1px solid var(--border)',
              }}>{t}</div>
            ))}
          </div>
        </div>
      )}

      <div className="card mt-16" style={{ borderColor: 'var(--warn)', background: '#d2992208' }}>
        <div style={{ fontWeight: 600, color: 'var(--warn)', marginBottom: 6 }}>⚠ Authorized Use Only</div>
        <div style={{ color: 'var(--muted)', fontSize: 13, lineHeight: 1.6 }}>
          Kestrel is an authorized security operations platform. Use only on systems you own or are
          explicitly permitted to test. All tool executions are logged for audit purposes.
        </div>
      </div>
    </div>
  )
}
