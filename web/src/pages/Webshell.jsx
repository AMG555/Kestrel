import React, { useState, useEffect, useRef } from 'react'
import { api } from '../api'

export default function Webshell() {
  const [connections, setConnections] = useState([])
  const [selectedConn, setSelectedConn] = useState(null)
  const [loading, setLoading] = useState(true)
  const [notice, setNotice] = useState(null)

  // Add modal
  const [showAddModal, setShowAddModal] = useState(false)
  const [newConn, setNewConn] = useState({
    name: '',
    url: '',
    password: 'pass',
    type: 'php',
    encoding: 'utf-8',
    os: 'linux',
  })
  const [saving, setSaving] = useState(false)

  // Terminal state
  const [command, setCommand] = useState('')
  const [terminalHistory, setTerminalHistory] = useState([])
  const [executing, setExecuting] = useState(false)
  const terminalBottomRef = useRef(null)

  useEffect(() => {
    loadConnections()
  }, [])

  useEffect(() => {
    terminalBottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [terminalHistory])

  async function loadConnections() {
    setLoading(true)
    try {
      const res = await api.listWebshells()
      const list = res.connections || []
      setConnections(list)
      if (list.length > 0 && !selectedConn) {
        setSelectedConn(list[0])
      }
    } catch (err) {
      setNotice({ type: 'error', text: `Failed to load connections: ${err.message}` })
    } finally {
      setLoading(false)
    }
  }

  async function handleAddConnection(e) {
    e.preventDefault()
    setSaving(true)
    try {
      const res = await api.createWebshell(newConn)
      setShowAddModal(false)
      setNewConn({ name: '', url: '', password: 'pass', type: 'php', encoding: 'utf-8', os: 'linux' })
      setNotice({ type: 'success', text: `Connection ${res.name} created.` })
      loadConnections()
    } catch (err) {
      setNotice({ type: 'error', text: `Error creating connection: ${err.message}` })
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id) {
    if (!window.confirm('Delete this WebShell connection?')) return
    try {
      await api.deleteWebshell(id)
      if (selectedConn?.id === id) setSelectedConn(null)
      loadConnections()
    } catch (err) {
      setNotice({ type: 'error', text: err.message })
    }
  }

  async function handleProbe(conn) {
    setNotice({ type: 'info', text: `Testing connection to ${conn.name}...` })
    try {
      const res = await api.probeWebshell(conn.id)
      if (res.connected) {
        setNotice({ type: 'success', text: `Connection active! Target responded successfully.` })
      } else {
        setNotice({ type: 'error', text: `Probe failed. Target returned non-success response.` })
      }
      loadConnections()
    } catch (err) {
      setNotice({ type: 'error', text: `Probe error: ${err.message}` })
    }
  }

  async function handleExec(e) {
    if (e) e.preventDefault()
    const trimmed = command.trim()
    if (!trimmed || !selectedConn || executing) return

    setExecuting(true)
    const runTime = new Date().toLocaleTimeString()
    try {
      const res = await api.execWebshell(selectedConn.id, { command: trimmed })
      setTerminalHistory(prev => [
        ...prev,
        {
          id: Date.now(),
          cmd: trimmed,
          output: res.output,
          durationMs: res.duration_ms,
          time: runTime,
        }
      ])
      setCommand('')
    } catch (err) {
      setTerminalHistory(prev => [
        ...prev,
        {
          id: Date.now(),
          cmd: trimmed,
          output: `Error: ${err.message}`,
          durationMs: 0,
          time: runTime,
        }
      ])
    } finally {
      setExecuting(false)
    }
  }

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0, display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>🐚</span> WebShell & Evaluator Management
          </h1>
          <p style={{ color: 'var(--muted)', fontSize: 13, margin: '4px 0 0' }}>
            Authorized target probe and post-exploitation virtual console for security evaluation.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={() => setShowAddModal(true)} className="primary" style={{ fontSize: 12 }}>
            + New Connection
          </button>
          <button onClick={loadConnections} disabled={loading} style={{ fontSize: 12 }}>
            Refresh
          </button>
        </div>
      </div>

      {notice && (
        <div style={{
          padding: '10px 14px',
          background: notice.type === 'error' ? 'rgba(248,81,73,0.1)' : notice.type === 'success' ? 'rgba(63,185,80,0.1)' : 'rgba(56,139,253,0.1)',
          border: `1px solid ${notice.type === 'error' ? 'var(--danger)' : notice.type === 'success' ? 'var(--success)' : 'var(--accent)'}`,
          borderRadius: 6,
          color: notice.type === 'error' ? 'var(--danger)' : notice.type === 'success' ? 'var(--success)' : 'var(--accent)',
          marginBottom: 16,
          fontSize: 13,
        }}>
          {notice.text}
        </div>
      )}

      {/* Main 2-column layout: Connections Sidebar vs Terminal */}
      <div style={{ display: 'grid', gridTemplateColumns: '340px 1fr', gap: 16, minHeight: 600 }}>
        {/* Connections List */}
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 16, display: 'flex', flexDirection: 'column' }}>
          <h2 style={{ fontSize: 14, fontWeight: 600, margin: '0 0 12px' }}>Registered Connections ({connections.length})</h2>
          {connections.length === 0 ? (
            <div style={{ color: 'var(--muted)', fontSize: 13, padding: '20px 0', textAlign: 'center' }}>
              No connections configured yet.
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, overflowY: 'auto', flex: 1 }}>
              {connections.map(conn => {
                const isSelected = selectedConn?.id === conn.id
                return (
                  <div
                    key={conn.id}
                    onClick={() => setSelectedConn(conn)}
                    style={{
                      padding: 12,
                      borderRadius: 6,
                      border: `1px solid ${isSelected ? 'var(--accent)' : 'var(--border)'}`,
                      background: isSelected ? 'var(--surface2)' : 'transparent',
                      cursor: 'pointer',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 4,
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span style={{ fontWeight: 600, fontSize: 13 }}>{conn.name}</span>
                      <span style={{
                        fontSize: 11,
                        padding: '1px 6px',
                        borderRadius: 4,
                        background: conn.status === 'connected' ? 'rgba(63,185,80,0.15)' : 'rgba(139,148,158,0.15)',
                        color: conn.status === 'connected' ? 'var(--success)' : 'var(--muted)',
                      }}>
                        {conn.status}
                      </span>
                    </div>
                    <div style={{ fontSize: 11, color: 'var(--muted)', fontFamily: 'monospace', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {conn.url}
                    </div>
                    <div style={{ display: 'flex', gap: 6, marginTop: 4, alignItems: 'center' }}>
                      <span style={{ fontSize: 10, background: 'rgba(255,255,255,0.06)', padding: '2px 5px', borderRadius: 3 }}>
                        {conn.type.toUpperCase()}
                      </span>
                      <span style={{ fontSize: 10, background: 'rgba(255,255,255,0.06)', padding: '2px 5px', borderRadius: 3 }}>
                        {conn.os}
                      </span>
                      <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
                        <button
                          type="button"
                          onClick={(e) => { e.stopPropagation(); handleProbe(conn); }}
                          style={{ fontSize: 10, padding: '2px 6px' }}
                        >
                          Probe
                        </button>
                        <button
                          type="button"
                          onClick={(e) => { e.stopPropagation(); handleDelete(conn.id); }}
                          className="danger"
                          style={{ fontSize: 10, padding: '2px 6px' }}
                        >
                          ✕
                        </button>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>

        {/* Interactive Virtual Terminal View */}
        <div style={{
          background: '#090d13',
          border: '1px solid var(--border)',
          borderRadius: 8,
          padding: 16,
          display: 'flex',
          flexDirection: 'column',
          height: 600,
          fontFamily: 'Consolas, monospace',
        }}>
          {/* Terminal Title Bar */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--border)', paddingBottom: 10, marginBottom: 12 }}>
            <div style={{ fontSize: 13, color: '#c9d1d9', display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ color: 'var(--accent)', fontWeight: 700 }}>Active Target:</span>
              <span>{selectedConn ? `${selectedConn.name} (${selectedConn.url})` : 'None selected'}</span>
            </div>
            {selectedConn && (
              <div style={{ display: 'flex', gap: 6 }}>
                {['id', 'whoami', 'uname -a', 'pwd'].map(cmd => (
                  <button
                    key={cmd}
                    onClick={() => setCommand(cmd)}
                    style={{ fontSize: 11, padding: '2px 6px', background: 'var(--surface)' }}
                  >
                    {cmd}
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Terminal Output Scroll Area */}
          <div style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 12, fontSize: 12 }}>
            {terminalHistory.length === 0 ? (
              <div style={{ color: 'var(--muted)', padding: '20px 0' }}>
                {selectedConn ? 'Target connected. Enter command below to execute.' : 'Select or add a connection from the left panel.'}
              </div>
            ) : (
              terminalHistory.map(item => (
                <div key={item.id} style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <div style={{ color: 'var(--accent)', display: 'flex', gap: 8 }}>
                    <span style={{ color: 'var(--success)' }}>shell&gt;</span>
                    <span style={{ color: '#fff', fontWeight: 600 }}>{item.cmd}</span>
                    <span style={{ marginLeft: 'auto', color: 'var(--muted)', fontSize: 11 }}>{item.time} ({item.durationMs}ms)</span>
                  </div>
                  <pre style={{ margin: 0, color: '#c9d1d9', whiteSpace: 'pre-wrap', wordBreak: 'break-all', background: 'rgba(255,255,255,0.02)', padding: 8, borderRadius: 4 }}>
                    {item.output || '(no output)'}
                  </pre>
                </div>
              ))
            )}
            <div ref={terminalBottomRef} />
          </div>

          {/* Terminal Input Bar */}
          <form onSubmit={handleExec} style={{ marginTop: 12, display: 'flex', gap: 8 }}>
            <span style={{ color: 'var(--success)', alignSelf: 'center', fontWeight: 700 }}>shell&gt;</span>
            <input
              type="text"
              placeholder={selectedConn ? "Enter shell command and press Enter..." : "Select connection first..."}
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              disabled={!selectedConn || executing}
              style={{
                flex: 1,
                fontFamily: 'Consolas, monospace',
                fontSize: 13,
                background: '#161b22',
                border: '1px solid var(--border)',
                color: '#fff',
                padding: '8px 12px',
                borderRadius: 4,
              }}
            />
            <button
              type="submit"
              className="primary"
              disabled={!selectedConn || executing || !command.trim()}
              style={{ padding: '8px 16px' }}
            >
              {executing ? 'Executing…' : 'Run'}
            </button>
          </form>
        </div>
      </div>

      {/* New Connection Modal */}
      {showAddModal && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
        }}>
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 24, width: 460 }}>
            <h3 style={{ margin: '0 0 16px', fontSize: 16 }}>Add Target WebShell Connection</h3>
            <form onSubmit={handleAddConnection} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div>
                <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Connection Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Test Lab Evaluator"
                  value={newConn.name}
                  onChange={(e) => setNewConn(prev => ({ ...prev, name: e.target.value }))}
                  style={{ width: '100%', padding: '8px' }}
                />
              </div>
              <div>
                <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Target URL</label>
                <input
                  type="url"
                  required
                  placeholder="https://target.local/eval.php"
                  value={newConn.url}
                  onChange={(e) => setNewConn(prev => ({ ...prev, url: e.target.value }))}
                  style={{ width: '100%', padding: '8px', fontFamily: 'monospace' }}
                />
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                <div>
                  <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Password / Param Key</label>
                  <input
                    type="text"
                    required
                    value={newConn.password}
                    onChange={(e) => setNewConn(prev => ({ ...prev, password: e.target.value }))}
                    style={{ width: '100%', padding: '8px', fontFamily: 'monospace' }}
                  />
                </div>
                <div>
                  <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Type</label>
                  <select
                    value={newConn.type}
                    onChange={(e) => setNewConn(prev => ({ ...prev, type: e.target.value }))}
                    style={{ width: '100%', padding: '8px' }}
                  >
                    <option value="php">PHP</option>
                    <option value="jsp">JSP</option>
                    <option value="aspx">ASPX</option>
                    <option value="custom">Custom HTTP Evaluator</option>
                  </select>
                </div>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                <div>
                  <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>OS</label>
                  <select
                    value={newConn.os}
                    onChange={(e) => setNewConn(prev => ({ ...prev, os: e.target.value }))}
                    style={{ width: '100%', padding: '8px' }}
                  >
                    <option value="linux">Linux</option>
                    <option value="windows">Windows</option>
                    <option value="auto">Auto</option>
                  </select>
                </div>
                <div>
                  <label style={{ fontSize: 12, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Encoding</label>
                  <select
                    value={newConn.encoding}
                    onChange={(e) => setNewConn(prev => ({ ...prev, encoding: e.target.value }))}
                    style={{ width: '100%', padding: '8px' }}
                  >
                    <option value="utf-8">UTF-8</option>
                    <option value="gbk">GBK</option>
                    <option value="auto">Auto</option>
                  </select>
                </div>
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 12 }}>
                <button type="button" onClick={() => setShowAddModal(false)}>Cancel</button>
                <button type="submit" className="primary" disabled={saving}>
                  {saving ? 'Saving…' : 'Save Connection'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
