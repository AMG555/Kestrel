import { useState, useEffect } from 'react'
import { api } from '../api.js'

// Read/write the theme preference from localStorage + document root class.
function getStoredTheme() {
  return localStorage.getItem('theme') || 'dark'
}
function applyTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme)
  localStorage.setItem('theme', theme)
}

export default function SettingsPage() {
  const [info, setInfo]         = useState(null)
  const [mcpServers, setMcpSvrs] = useState([])
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState('')
  const [showAddMcp, setShowAddMcp] = useState(false)
  const [mcpForm, setMcpForm]   = useState({ name: '', transport: 'http', url: '', timeout_seconds: 120 })
  const [saving, setSaving]     = useState(false)
  const [saveMsg, setSaveMsg]   = useState('')
  const [theme, setTheme]       = useState(getStoredTheme)
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw]       = useState('')
  const [confirmPw, setConfirmPw] = useState('')
  const [pwError, setPwError]   = useState('')
  const [pwSuccess, setPwSuccess] = useState('')
  const [pwSaving, setPwSaving] = useState(false)

  const handlePasswordChange = async (e) => {
    e.preventDefault()
    setPwError('')
    setPwSuccess('')
    if (newPw.length < 8) {
      setPwError('New password must be at least 8 characters')
      return
    }
    if (newPw !== confirmPw) {
      setPwError('New passwords do not match')
      return
    }
    setPwSaving(true)
    try {
      await api.changePassword(currentPw, newPw)
      setPwSuccess('Password changed successfully')
      setCurrentPw('')
      setNewPw('')
      setConfirmPw('')
    } catch (err) {
      setPwError(err.message || 'Failed to change password')
    } finally {
      setPwSaving(false)
    }
  }

  const toggleTheme = () => {
    const next = theme === 'dark' ? 'light' : 'dark'
    setTheme(next)
    applyTheme(next)
  }

  const load = () => {
    setLoading(true)
    Promise.all([api.systemInfo(), api.listMCPServers()])
      .then(([i, m]) => {
        const svrs = Array.isArray(m?.servers)
          ? m.servers
          : Object.entries(m?.servers || {}).map(([name, s]) => ({
              id: name,
              name,
              transport: s?.config?.transport || 'http',
              url: s?.config?.url || s?.config?.command || '',
              status: s?.status || 'disconnected',
              circuit_open: s?.status === 'circuit_open' || s?.status === 'error',
            }))
        setMcpSvrs(svrs)
        setLoading(false)
      })
      .catch(e => { setError(e.message); setLoading(false) })
  }

  useEffect(() => { load() }, [])

  const handleAddMcp = async (e) => {
    e.preventDefault()
    setSaving(true)
    try {
      await api.upsertMCPServer(mcpForm)
      setShowAddMcp(false)
      setMcpForm({ name: '', transport: 'http', url: '', timeout_seconds: 120 })
      setSaveMsg('MCP server added')
      load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
      setTimeout(() => setSaveMsg(''), 3000)
    }
  }

  const handleDeleteMcp = async (id) => {
    if (!confirm('Remove this MCP server?')) return
    await api.deleteMCPServer(id).catch(e => setError(e.message))
    load()
  }

  const handleResetCircuit = async (id) => {
    await api.resetCircuit(id).catch(e => setError(e.message))
    setSaveMsg('Circuit reset')
    setTimeout(() => setSaveMsg(''), 2000)
    load()
  }

  const LLM_PROVIDERS = [
    { name: 'OpenAI', provider: 'openai', baseUrl: 'https://api.openai.com/v1', models: ['gpt-4o', 'gpt-4o-mini', 'gpt-4-turbo', 'gpt-3.5-turbo'] },
    { name: 'Anthropic', provider: 'anthropic', baseUrl: 'https://api.anthropic.com', models: ['claude-3-5-sonnet-20241022', 'claude-3-5-haiku-20241022', 'claude-3-opus-20240229'] },
    { name: 'Ollama (local)', provider: 'ollama', baseUrl: 'http://localhost:11434', models: ['llama3.2', 'llama3.1', 'mistral', 'codestral'] },
  ]

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 }}>
        <h2 style={{ margin: 0 }}>Settings</h2>
        <button
          onClick={toggleTheme}
          style={{ fontSize: 13, padding: '6px 14px', minWidth: 120 }}
          title="Toggle light/dark mode"
        >
          {theme === 'dark' ? '☀ Light mode' : '🌙 Dark mode'}
        </button>
      </div>

      {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}
      {saveMsg && <div style={{ background: '#22c55e22', color: '#22c55e', border: '1px solid #22c55e55', borderRadius: 6, padding: '8px 12px', marginBottom: 12, fontSize: 13 }}>{saveMsg}</div>}

      {loading ? (
        <div style={{ color: 'var(--muted)' }}>Loading…</div>
      ) : (
        <div style={{ display: 'grid', gap: 24 }}>

          {/* Platform info */}
          <section style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 20 }}>
            <h3 style={{ margin: '0 0 16px', fontSize: 15 }}>Platform</h3>
            <table style={{ fontSize: 13, borderCollapse: 'collapse' }}>
              <tbody>
                {[
                  ['Version', info?.version],
                  ['Status', info?.status],
                  ['Agent modes', info?.agent_modes?.join(', ')],
                  ['HITL modes', info?.hitl_modes?.join(', ')],
                  ['LLM provider', info?.llm_provider || 'stub (rule-based)'],
                ].map(([k, v]) => (
                  <tr key={k}>
                    <td style={{ color: 'var(--muted)', width: 180, padding: '4px 0' }}>{k}</td>
                    <td style={{ fontWeight: 500 }}>{v || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>

          {/* Built-in recon tools */}
          <section style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 20 }}>
            <h3 style={{ margin: '0 0 16px', fontSize: 15 }}>Built-in Recon Tools</h3>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 8 }}>
              {(info?.tools || []).map(t => (
                <div key={t} style={{
                  background: 'var(--surface2)', borderRadius: 6, padding: '8px 12px',
                  fontSize: 12, border: '1px solid var(--border)',
                }}>
                  <div style={{ fontWeight: 600, marginBottom: 2 }}>{t}</div>
                  <div style={{ fontSize: 10, color: '#22c55e' }}>read-only</div>
                </div>
              ))}
            </div>
          </section>

          {/* LLM Configuration guidance */}
          <section style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 20 }}>
            <h3 style={{ margin: '0 0 12px', fontSize: 15 }}>LLM Configuration</h3>
            <p style={{ color: 'var(--muted)', fontSize: 13, margin: '0 0 16px', lineHeight: 1.6 }}>
              Configure LLM providers in <code style={{ background: 'var(--surface2)', padding: '1px 5px', borderRadius: 3 }}>config.yaml</code>.
              Kestrel runs in rule-based stub mode when no provider is configured.
            </p>
            <div style={{ display: 'grid', gap: 12 }}>
              {LLM_PROVIDERS.map(p => (
                <div key={p.provider} style={{
                  background: 'var(--surface2)', borderRadius: 6, padding: '12px 14px',
                  border: '1px solid var(--border)',
                }}>
                  <div style={{ fontWeight: 600, fontSize: 13, marginBottom: 6 }}>{p.name}</div>
                  <pre style={{ fontSize: 11, margin: 0, color: 'var(--muted)', lineHeight: 1.7 }}>{
`ai:
  default_channel: ${p.provider}
  channels:
    ${p.provider}:
      provider: ${p.provider}
      api_key: \${${p.provider.toUpperCase()}_API_KEY}
      base_url: "${p.baseUrl}"
      model: "${p.models[0]}"`
                  }</pre>
                </div>
              ))}
            </div>
          </section>

          {/* MCP Servers */}
          <section style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 20 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <h3 style={{ margin: 0, fontSize: 15 }}>External MCP Servers</h3>
              <button onClick={() => setShowAddMcp(v => !v)} style={{ fontSize: 12 }}>
                {showAddMcp ? 'Cancel' : '+ Add Server'}
              </button>
            </div>

            {showAddMcp && (
              <form onSubmit={handleAddMcp} style={{ background: 'var(--surface2)', borderRadius: 8, padding: 14, marginBottom: 16, display: 'grid', gap: 10 }}>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                  <div>
                    <label style={{ fontSize: 11, color: 'var(--muted)' }}>Name *</label>
                    <input value={mcpForm.name} required onChange={e => setMcpForm(f => ({ ...f, name: e.target.value }))}
                      style={{ width: '100%', marginTop: 4 }} placeholder="my-recon-server" />
                  </div>
                  <div>
                    <label style={{ fontSize: 11, color: 'var(--muted)' }}>Transport</label>
                    <select value={mcpForm.transport} onChange={e => setMcpForm(f => ({ ...f, transport: e.target.value }))}
                      style={{ width: '100%', marginTop: 4 }}>
                      <option value="http">HTTP</option>
                      <option value="stdio">stdio</option>
                      <option value="sse">SSE</option>
                    </select>
                  </div>
                </div>
                <div>
                  <label style={{ fontSize: 11, color: 'var(--muted)' }}>URL / Endpoint</label>
                  <input value={mcpForm.url} onChange={e => setMcpForm(f => ({ ...f, url: e.target.value }))}
                    style={{ width: '100%', marginTop: 4 }} placeholder="http://localhost:9000/mcp" />
                </div>
                <div style={{ textAlign: 'right' }}>
                  <button type="submit" disabled={saving} className="primary">{saving ? 'Saving…' : 'Add Server'}</button>
                </div>
              </form>
            )}

            {mcpServers.length === 0 ? (
              <div style={{ color: 'var(--muted)', fontSize: 13 }}>No external MCP servers registered.</div>
            ) : (
              <div style={{ display: 'grid', gap: 8 }}>
                {mcpServers.map(s => (
                  <div key={s.id} style={{
                    background: 'var(--surface2)', borderRadius: 6, padding: '10px 14px',
                    display: 'flex', alignItems: 'center', gap: 12,
                  }}>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontWeight: 600, fontSize: 13 }}>{s.name}</div>
                      <div style={{ fontSize: 11, color: 'var(--muted)' }}>
                        {s.transport} · {s.url || '(no URL)'}
                        {s.circuit_open ? <span style={{ color: 'var(--danger)', marginLeft: 8 }}>⚡ circuit open</span> : null}
                      </div>
                    </div>
                    <div style={{ display: 'flex', gap: 6 }}>
                      {s.circuit_open && (
                        <button onClick={() => handleResetCircuit(s.id)} style={{ fontSize: 11 }}>Reset Circuit</button>
                      )}
                      <button onClick={() => handleDeleteMcp(s.id)}
                        style={{ fontSize: 11, color: 'var(--danger)' }}>Remove</button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>

          {/* Security / Password Change */}
          <section style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 20 }}>
            <h3 style={{ margin: '0 0 16px 0', fontSize: 15 }}>Security & Credentials</h3>
            <form onSubmit={handlePasswordChange} style={{ display: 'grid', gap: 12, maxWidth: 400 }}>
              <div>
                <label style={{ fontSize: 11, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Current Password</label>
                <input type="password" required value={currentPw} onChange={e => setCurrentPw(e.target.value)} style={{ width: '100%' }} />
              </div>
              <div>
                <label style={{ fontSize: 11, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>New Password (min 8 characters)</label>
                <input type="password" required value={newPw} onChange={e => setNewPw(e.target.value)} style={{ width: '100%' }} />
              </div>
              <div>
                <label style={{ fontSize: 11, color: 'var(--muted)', display: 'block', marginBottom: 4 }}>Confirm New Password</label>
                <input type="password" required value={confirmPw} onChange={e => setConfirmPw(e.target.value)} style={{ width: '100%' }} />
              </div>
              {pwError && <div style={{ color: 'var(--danger)', fontSize: 12 }}>{pwError}</div>}
              {pwSuccess && <div style={{ color: '#2ea043', fontSize: 12 }}>{pwSuccess}</div>}
              <div>
                <button type="submit" disabled={pwSaving} className="primary" style={{ fontSize: 12 }}>
                  {pwSaving ? 'Updating…' : 'Update Password'}
                </button>
              </div>
            </form>
          </section>

          {/* Disclaimer */}
          <section style={{ background: '#d2992208', border: '1px solid var(--warn)', borderRadius: 8, padding: 16 }}>
            <div style={{ fontWeight: 600, color: 'var(--warn)', marginBottom: 6 }}>⚠ Authorized Use Only</div>
            <div style={{ color: 'var(--muted)', fontSize: 13, lineHeight: 1.6 }}>
              Kestrel is an authorized security operations platform. Use only on systems you own or
              are explicitly permitted to test. All tool executions and operations are governed by
              RBAC authorization controls and recorded to an immutable audit trail.
            </div>
          </section>

        </div>
      )}
    </div>
  )
}
