import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api.js'

function StatCard({ label, value, color, to }) {
  const inner = (
    <div className="card" style={{ textAlign: 'center', padding: '20px 16px', cursor: to ? 'pointer' : 'default' }}>
      <div style={{ fontSize: 32, fontWeight: 700, color: color || 'var(--accent)' }}>{value ?? '—'}</div>
      <div style={{ color: 'var(--muted)', fontSize: 12, marginTop: 4 }}>{label}</div>
    </div>
  )
  if (to) return <Link to={to} style={{ textDecoration: 'none' }}>{inner}</Link>
  return inner
}

function TrendBar({ trend }) {
  if (!trend || trend.length === 0) {
    return <div style={{ color: 'var(--muted)', fontSize: 12 }}>No data in the last 7 days.</div>
  }
  const max = Math.max(...trend.map(t => t.count), 1)
  return (
    <div style={{ display: 'flex', alignItems: 'flex-end', gap: 4, height: 48 }}>
      {trend.map(t => (
        <div key={t.day} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
          <div style={{
            width: '100%', background: 'var(--accent)',
            height: `${Math.round((t.count / max) * 40)}px`,
            borderRadius: '2px 2px 0 0', minHeight: 2, opacity: 0.8,
          }} title={`${t.day}: ${t.count}`} />
          <div style={{ fontSize: 8, color: 'var(--muted)', transform: 'rotate(-45deg)', whiteSpace: 'nowrap' }}>
            {t.day?.slice(5)}
          </div>
        </div>
      ))}
    </div>
  )
}

export default function Dashboard() {
  const [stats, setStats] = useState(null)
  const [info, setInfo]   = useState(null)
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

      {/* Primary stats row */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(150px,1fr))', gap: 12, marginBottom: 16 }}>
        <StatCard label="Projects"          value={stats?.projects}         color="var(--accent)"  to="/projects" />
        <StatCard label="Agent Sessions"    value={stats?.agent_sessions}   color="#8b5cf6"        to="/sessions" />
        <StatCard label="Tool Executions"   value={stats?.tool_executions}                         to="/tool-executions" />
        <StatCard label="Assets"            value={stats?.assets}           color="var(--success)" to="/assets" />
        <StatCard label="Vulnerabilities"   value={stats?.vulnerabilities}  color="var(--danger)"  to="/vulns" />
        <StatCard label="Tools Available"   value={stats?.tools_available}  color="var(--accent)" />
      </div>

      {/* Alert row */}
      {stats && (stats.hitl_pending > 0 || stats.critical_vulns > 0 || stats.high_vulns > 0) && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(150px,1fr))', gap: 12, marginBottom: 20 }}>
          {stats.hitl_pending > 0 && (
            <StatCard label="Pending Approvals" value={stats.hitl_pending} color="var(--warn)" to="/hitl" />
          )}
          {stats.critical_vulns > 0 && (
            <StatCard label="Critical (open)" value={stats.critical_vulns} color="var(--danger)" to="/vulns" />
          )}
          {stats.high_vulns > 0 && (
            <StatCard label="High (open)" value={stats.high_vulns} color="#f97316" to="/vulns" />
          )}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        {/* Tool trend */}
        <div className="card">
          <div style={{ fontWeight: 600, marginBottom: 12, fontSize: 13 }}>Tool Executions — last 7 days</div>
          <TrendBar trend={stats?.tool_exec_trend} />
        </div>

        {/* Platform status */}
        {info && (
          <div className="card">
            <div style={{ fontWeight: 600, marginBottom: 12, fontSize: 13 }}>Platform Status</div>
            <table>
              <tbody>
                <tr><td style={{ color: 'var(--muted)', width: 130 }}>Version</td><td>{info.version}</td></tr>
                <tr><td style={{ color: 'var(--muted)' }}>Status</td>
                  <td><span className="badge badge-medium">{info.status}</span></td></tr>
                <tr><td style={{ color: 'var(--muted)' }}>Agent Modes</td><td style={{ fontSize: 12 }}>{info.agent_modes?.join(', ')}</td></tr>
                <tr><td style={{ color: 'var(--muted)' }}>HITL Modes</td><td style={{ fontSize: 12 }}>{info.hitl_modes?.join(', ')}</td></tr>
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Tools grid */}
      {info && (
        <div className="card mt-16">
          <div style={{ fontWeight: 600, marginBottom: 12, fontSize: 13 }}>Available Recon Tools</div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {info.tools?.map(t => (
              <div key={t} style={{
                padding: '4px 12px', background: 'var(--surface2)',
                borderRadius: 12, fontSize: 12, border: '1px solid var(--border)',
              }}>{t}</div>
            ))}
          </div>
        </div>
      )}

      <div className="card mt-16" style={{ borderColor: 'var(--warn)', background: '#d2992208' }}>
        <div style={{ fontWeight: 600, color: 'var(--warn)', marginBottom: 6 }}>⚠ Authorized Use Only</div>
        <div style={{ color: 'var(--muted)', fontSize: 13, lineHeight: 1.6 }}>
          Kestrel is an authorized security operations platform. Use only on systems you own or are
          explicitly permitted to test. All tool executions are logged to an append-only audit trail.
        </div>
      </div>
    </div>
  )
}
