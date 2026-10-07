import React, { useState, useEffect } from 'react'
import { api } from '../api'

export default function Monitor() {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [lastUpdated, setLastUpdated] = useState(null)
  const [error, setError] = useState(null)

  useEffect(() => {
    fetchStatus()
  }, [])

  useEffect(() => {
    if (!autoRefresh) return
    const timer = setInterval(() => {
      fetchStatus(false)
    }, 5000)
    return () => clearInterval(timer)
  }, [autoRefresh])

  async function fetchStatus(showLoading = true) {
    if (showLoading) setLoading(true)
    try {
      const res = await api.getMonitorStatus()
      setData(res)
      setError(null)
      setLastUpdated(new Date().toLocaleTimeString())
    } catch (err) {
      setError(err.message)
    } finally {
      if (showLoading) setLoading(false)
    }
  }

  function formatUptime(seconds) {
    if (!seconds) return '0s'
    const d = Math.floor(seconds / (3600 * 24))
    const h = Math.floor((seconds % (3600 * 24)) / 3600)
    const m = Math.floor((seconds % 3600) / 60)
    const s = Math.floor(seconds % 60)
    const parts = []
    if (d > 0) parts.push(`${d}d`)
    if (h > 0) parts.push(`${h}h`)
    if (m > 0) parts.push(`${m}m`)
    parts.push(`${s}s`)
    return parts.join(' ')
  }

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0, display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>📈</span> System Telemetry & Health
          </h1>
          <p style={{ color: 'var(--muted)', fontSize: 13, margin: '4px 0 0' }}>
            Real-time metrics, goroutines, memory usage, and persistent storage telemetry.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--muted)', cursor: 'pointer' }}>
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
            />
            Auto-refresh (5s)
          </label>
          <button onClick={() => fetchStatus(true)} disabled={loading} style={{ fontSize: 12, padding: '5px 12px' }}>
            {loading ? 'Refreshing…' : 'Refresh Now'}
          </button>
        </div>
      </div>

      {error && (
        <div style={{
          padding: '12px 16px',
          background: 'rgba(248,81,73,0.1)',
          border: '1px solid var(--danger)',
          borderRadius: 6,
          color: 'var(--danger)',
          marginBottom: 20,
        }}>
          ⚠️ Connection error: {error}
        </div>
      )}

      {/* Top Banner Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 16, marginBottom: 24 }}>
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 16 }}>
          <div style={{ fontSize: 12, color: 'var(--muted)' }}>Health Status</div>
          <div style={{ fontSize: 20, fontWeight: 700, color: 'var(--success)', marginTop: 4, display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{ display: 'inline-block', width: 10, height: 10, borderRadius: '50%', background: 'var(--success)' }}></span>
            {data?.status?.toUpperCase() || 'UNKNOWN'}
          </div>
          <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 4 }}>Last checked: {lastUpdated || 'Never'}</div>
        </div>

        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 16 }}>
          <div style={{ fontSize: 12, color: 'var(--muted)' }}>Process Uptime</div>
          <div style={{ fontSize: 20, fontWeight: 700, color: 'var(--accent)', marginTop: 4 }}>
            {formatUptime(data?.system?.uptime_secs)}
          </div>
          <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 4 }}>Continuous server execution</div>
        </div>

        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 16 }}>
          <div style={{ fontSize: 12, color: 'var(--muted)' }}>Active Goroutines</div>
          <div style={{ fontSize: 20, fontWeight: 700, color: 'var(--text)', marginTop: 4 }}>
            {data?.system?.goroutines ?? '—'}
          </div>
          <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 4 }}>Concurrent runtime workers</div>
        </div>

        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 16 }}>
          <div style={{ fontSize: 12, color: 'var(--muted)' }}>Tool Recipes Registered</div>
          <div style={{ fontSize: 20, fontWeight: 700, color: '#a371f7', marginTop: 4 }}>
            {data?.tools?.registered_count ?? '—'}
          </div>
          <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 4 }}>Security executors available</div>
        </div>
      </div>

      {/* Main Grid: Runtime System & Database Storage */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(400px, 1fr))', gap: 20 }}>
        {/* System & Go Runtime */}
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 20 }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, margin: '0 0 16px', display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>⚙️</span> Go Runtime Environment
          </h2>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <tbody>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Operating System</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600 }}>{data?.system?.os} ({data?.system?.arch})</td>
              </tr>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Go Toolchain</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontFamily: 'monospace' }}>{data?.system?.go_version}</td>
              </tr>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>CPU Cores Available</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600 }}>{data?.system?.num_cpu} logical cores</td>
              </tr>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Heap Memory Allocated</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600, color: 'var(--accent)' }}>
                  {data?.system?.memory_alloc_mb?.toFixed(2)} MB
                </td>
              </tr>
              <tr>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>System Memory Reserved</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600 }}>
                  {data?.system?.memory_sys_mb?.toFixed(2)} MB
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        {/* Database & Platform Records */}
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 20 }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, margin: '0 0 16px', display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>🗄️</span> SQLite Storage & Entity Inventory
          </h2>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <tbody>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Database File Size</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600, color: 'var(--warn)' }}>
                  {data?.database?.size_mb?.toFixed(2)} MB ({data?.database?.size_bytes?.toLocaleString()} bytes)
                </td>
              </tr>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Total Users</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600 }}>{data?.database?.users}</td>
              </tr>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Active Testing Projects</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600 }}>{data?.database?.projects}</td>
              </tr>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Target Assets Tracked</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600 }}>{data?.database?.assets}</td>
              </tr>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Vulnerability Findings</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600, color: 'var(--danger)' }}>
                  {data?.database?.vulnerabilities}
                </td>
              </tr>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Agent Autonomous Sessions</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600 }}>{data?.database?.sessions}</td>
              </tr>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Recorded Tool Executions</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600 }}>{data?.database?.tool_executions}</td>
              </tr>
              <tr>
                <td style={{ padding: '8px 0', color: 'var(--muted)' }}>Audit Trail Log Entries</td>
                <td style={{ padding: '8px 0', textAlign: 'right', fontWeight: 600 }}>{data?.database?.audit_logs}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
