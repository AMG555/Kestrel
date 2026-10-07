import React, { useState, useEffect } from 'react'
import { api } from '../api'

export default function C2() {
  const [listeners, setListeners] = useState([])
  const [beacons, setBeacons] = useState([])
  const [loading, setLoading] = useState(true)
  const [notice, setNotice] = useState(null)

  // New listener modal
  const [showNewListener, setShowNewListener] = useState(false)
  const [newListener, setNewListener] = useState({ name: '', protocol: 'http', bind_host: '0.0.0.0', bind_port: 8443 })
  const [savingListener, setSavingListener] = useState(false)

  // Task beacon modal
  const [selectedBeacon, setSelectedBeacon] = useState(null)
  const [taskType, setTaskType] = useState('shell')
  const [taskPayload, setTaskPayload] = useState('')
  const [queuingTask, setQueuingTask] = useState(false)
  const [taskOutput, setTaskOutput] = useState(null)

  useEffect(() => {
    loadData()
  }, [])

  async function loadData() {
    setLoading(true)
    try {
      const [lRes, bRes] = await Promise.all([
        api.listC2Listeners().catch(() => ({ listeners: [] })),
        api.listC2Beacons().catch(() => ({ beacons: [] })),
      ])
      setListeners(lRes.listeners || [])
      setBeacons(bRes.beacons || [])
    } catch (err) {
      setNotice({ type: 'error', text: err.message })
    } finally {
      setLoading(false)
    }
  }

  async function handleCreateListener(e) {
    e.preventDefault()
    setSavingListener(true)
    try {
      await api.createC2Listener({
        ...newListener,
        bind_port: parseInt(newListener.bind_port, 10),
      })
      setShowNewListener(false)
      setNewListener({ name: '', protocol: 'http', bind_host: '0.0.0.0', bind_port: 8443 })
      setNotice({ type: 'success', text: 'Listener spawned successfully.' })
      loadData()
    } catch (err) {
      setNotice({ type: 'error', text: `Failed to create listener: ${err.message}` })
    } finally {
      setSavingListener(false)
    }
  }

  async function handleQueueTask(e) {
    e.preventDefault()
    if (!selectedBeacon) return
    setQueuingTask(true)
    try {
      const task = await api.queueC2Task(selectedBeacon.id, {
        type: taskType,
        payload: taskPayload,
      })
      setTaskOutput(`Task ${task.id} queued successfully (status: ${task.status}).`)
      setTaskPayload('')
    } catch (err) {
      setTaskOutput(`Error queuing task: ${err.message}`)
    } finally {
      setQueuingTask(false)
    }
  }

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0, display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>📡</span> Adversary Emulation & C2 Listeners
          </h1>
          <p style={{ color: 'var(--muted)', fontSize: 13, margin: '4px 0 0' }}>
            Simulate command-and-control ingress channels and evaluate post-exploitation detection capabilities.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={() => setShowNewListener(true)} className="primary" style={{ fontSize: 12 }}>
            + New Listener
          </button>
          <button onClick={loadData} disabled={loading} style={{ fontSize: 12 }}>
            Refresh
          </button>
        </div>
      </div>

      {/* Authorized scope notice */}
      <div style={{
        padding: '10px 14px',
        background: 'rgba(56, 139, 253, 0.08)',
        border: '1px solid rgba(56, 139, 253, 0.25)',
        borderRadius: 6,
        color: '#58a6ff',
        fontSize: 12,
        marginBottom: 20,
      }}>
        ℹ️ <strong>Authorized Red Team / Detection Engineering Scope:</strong> Listeners and beacon handlers are designed for controlled breach & attack simulation (BAS) to validate SIEM/EDR detection rules.
      </div>

      {notice && (
        <div style={{
          padding: '10px 14px',
          background: notice.type === 'error' ? 'rgba(248,81,73,0.1)' : 'rgba(63,185,80,0.1)',
          border: `1px solid ${notice.type === 'error' ? 'var(--danger)' : 'var(--success)'}`,
          borderRadius: 6,
          color: notice.type === 'error' ? 'var(--danger)' : 'var(--success)',
          marginBottom: 16,
          fontSize: 13,
        }}>
          {notice.text}
        </div>
      )}

      {/* Listeners Section */}
      <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 18, marginBottom: 24 }}>
        <h2 style={{ fontSize: 16, fontWeight: 600, margin: '0 0 12px' }}>Active Ingress Listeners</h2>
        {listeners.length === 0 ? (
          <div style={{ color: 'var(--muted)', fontSize: 13, padding: '12px 0' }}>No active listeners configured.</div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--muted)', textAlign: 'left' }}>
                <th style={{ padding: '8px 12px' }}>Name</th>
                <th style={{ padding: '8px 12px' }}>Protocol</th>
                <th style={{ padding: '8px 12px' }}>Bind Address</th>
                <th style={{ padding: '8px 12px' }}>Status</th>
                <th style={{ padding: '8px 12px' }}>Created</th>
              </tr>
            </thead>
            <tbody>
              {listeners.map(l => (
                <tr key={l.id} style={{ borderBottom: '1px solid var(--border)' }}>
                  <td style={{ padding: '10px 12px', fontWeight: 600 }}>{l.name}</td>
                  <td style={{ padding: '10px 12px' }}>
                    <span style={{ textTransform: 'uppercase', fontFamily: 'monospace', fontSize: 11, background: 'var(--surface2)', padding: '2px 6px', borderRadius: 4 }}>
                      {l.protocol}
                    </span>
                  </td>
                  <td style={{ padding: '10px 12px', fontFamily: 'monospace' }}>{l.bind_host}:{l.bind_port}</td>
                  <td style={{ padding: '10px 12px' }}>
                    <span style={{
                      color: l.status === 'active' ? 'var(--success)' : 'var(--muted)',
                      fontWeight: 600,
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 4,
                    }}>
                      <span style={{ width: 6, height: 6, borderRadius: '50%', background: l.status === 'active' ? 'var(--success)' : 'var(--muted)' }} />
                      {l.status}
                    </span>
                  </td>
                  <td style={{ padding: '10px 12px', color: 'var(--muted)', fontSize: 12 }}>
                    {new Date(l.created_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Target Beacons Section */}
      <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 18 }}>
        <h2 style={{ fontSize: 16, fontWeight: 600, margin: '0 0 12px' }}>Target Simulation Beacons</h2>
        {beacons.length === 0 ? (
          <div style={{ color: 'var(--muted)', fontSize: 13, padding: '12px 0' }}>
            No target beacons currently connected. Start an authorized agent or test payload to register.
          </div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--muted)', textAlign: 'left' }}>
                <th style={{ padding: '8px 12px' }}>Host / IP</th>
                <th style={{ padding: '8px 12px' }}>OS</th>
                <th style={{ padding: '8px 12px' }}>User</th>
                <th style={{ padding: '8px 12px' }}>PID</th>
                <th style={{ padding: '8px 12px' }}>Sleep</th>
                <th style={{ padding: '8px 12px' }}>Status</th>
                <th style={{ padding: '8px 12px' }}>Action</th>
              </tr>
            </thead>
            <tbody>
              {beacons.map(b => (
                <tr key={b.id} style={{ borderBottom: '1px solid var(--border)' }}>
                  <td style={{ padding: '10px 12px', fontWeight: 600 }}>{b.hostname} ({b.ip})</td>
                  <td style={{ padding: '10px 12px' }}>{b.os}</td>
                  <td style={{ padding: '10px 12px', fontFamily: 'monospace' }}>{b.user}</td>
                  <td style={{ padding: '10px 12px', fontFamily: 'monospace' }}>{b.pid}</td>
                  <td style={{ padding: '10px 12px' }}>{b.sleep_sec}s</td>
                  <td style={{ padding: '10px 12px' }}>
                    <span style={{ color: b.status === 'alive' ? 'var(--success)' : 'var(--danger)', fontWeight: 600 }}>
                      {b.status}
                    </span>
                  </td>
                  <td style={{ padding: '10px 12px' }}>
                    <button onClick={() => setSelectedBeacon(b)} style={{ fontSize: 12, padding: '3px 8px' }}>
                      Interact
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* New Listener Modal */}
      {showNewListener && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
        }}>
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 24, width: 440 }}>
            <h3 style={{ margin: '0 0 16px', fontSize: 16 }}>Create Emulation Listener</h3>
            <form onSubmit={handleCreateListener} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div>
                <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Listener Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Internal DevSecOps Emulation"
                  value={newListener.name}
                  onChange={(e) => setNewListener(prev => ({ ...prev, name: e.target.value }))}
                  style={{ width: '100%', padding: '8px' }}
                />
              </div>
              <div>
                <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Protocol</label>
                <select
                  value={newListener.protocol}
                  onChange={(e) => setNewListener(prev => ({ ...prev, protocol: e.target.value }))}
                  style={{ width: '100%', padding: '8px' }}
                >
                  <option value="http">HTTP</option>
                  <option value="tcp">Raw TCP</option>
                  <option value="ws">WebSocket</option>
                </select>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 8 }}>
                <div>
                  <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Bind Host</label>
                  <input
                    type="text"
                    value={newListener.bind_host}
                    onChange={(e) => setNewListener(prev => ({ ...prev, bind_host: e.target.value }))}
                    style={{ width: '100%', padding: '8px' }}
                  />
                </div>
                <div>
                  <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Port</label>
                  <input
                    type="number"
                    required
                    value={newListener.bind_port}
                    onChange={(e) => setNewListener(prev => ({ ...prev, bind_port: e.target.value }))}
                    style={{ width: '100%', padding: '8px' }}
                  />
                </div>
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 12 }}>
                <button type="button" onClick={() => setShowNewListener(false)}>Cancel</button>
                <button type="submit" className="primary" disabled={savingListener}>
                  {savingListener ? 'Creating…' : 'Start Listener'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Interact Beacon Modal */}
      {selectedBeacon && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
        }}>
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 24, width: 520 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <h3 style={{ margin: 0, fontSize: 16 }}>Interact: {selectedBeacon.hostname}</h3>
              <button onClick={() => { setSelectedBeacon(null); setTaskOutput(null); }} style={{ fontSize: 12, padding: '2px 8px' }}>✕</button>
            </div>
            <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 12 }}>
              ID: {selectedBeacon.id} | User: {selectedBeacon.user} | IP: {selectedBeacon.ip}
            </div>
            <form onSubmit={handleQueueTask} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div>
                <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Task Type</label>
                <select
                  value={taskType}
                  onChange={(e) => setTaskType(e.target.value)}
                  style={{ width: '100%', padding: '8px' }}
                >
                  <option value="shell">Shell Command</option>
                  <option value="sysinfo">System Info</option>
                  <option value="sleep">Update Sleep Interval</option>
                  <option value="exit">Terminate Beacon</option>
                </select>
              </div>
              <div>
                <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Payload / Parameters</label>
                <input
                  type="text"
                  required
                  placeholder={taskType === 'shell' ? 'e.g. id || whoami' : 'Parameters...'}
                  value={taskPayload}
                  onChange={(e) => setTaskPayload(e.target.value)}
                  style={{ width: '100%', padding: '8px', fontFamily: 'monospace' }}
                />
              </div>
              {taskOutput && (
                <div style={{ background: '#090d13', padding: 10, borderRadius: 6, fontSize: 12, fontFamily: 'monospace', color: 'var(--success)' }}>
                  {taskOutput}
                </div>
              )}
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 8 }}>
                <button type="submit" className="primary" disabled={queuingTask}>
                  {queuingTask ? 'Dispatching…' : 'Dispatch Task'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
