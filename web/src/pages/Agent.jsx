import { useState, useEffect, useRef } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, connectAgentStream } from '../api.js'

export default function AgentPage() {
  const [searchParams]          = useSearchParams()
  const [tools, setTools]       = useState([])
  const [skills, setSkills]     = useState([])
  const [sidebarTab, setSidebarTab] = useState('tools') // 'tools' | 'skills'
  const [catalogSearch, setCatalogSearch] = useState('')
  const [projects, setProjects] = useState([])
  const [intent, setIntent]     = useState(searchParams.get('intent') || '')
  const [projectID, setProjectID] = useState(searchParams.get('project') || '')
  const [title, setTitle]       = useState('')
  const [mode, setMode]         = useState(searchParams.get('mode') || 'single')
  const [hitlMode, setHitlMode] = useState('auto')
  const [events, setEvents]     = useState([])
  const [running, setRunning]   = useState(false)
  const [wsRef, setWsRef]       = useState(null)
  const [sessionID, setSessionID] = useState(null)
  const [copied, setCopied]     = useState(false)
  const logRef = useRef(null)

  useEffect(() => {
    api.listTools().then(d => setTools(d.tools || []))
    api.listSkills().then(d => setSkills(d.skills || [])).catch(() => {})
    api.listProjects().then(d => setProjects(d.projects || [])).catch(() => {})
  }, [])

  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight
  }, [events])

  const run = () => {
    if (!intent.trim() || running) return
    setEvents([])
    setSessionID(null)
    setRunning(true)

    const ws = connectAgentStream(
      (ev) => {
        setEvents(prev => [...prev, ev])
        if (ev.type === 'final' || ev.type === 'error') setRunning(false)
        // Capture session_id from first event carrying it.
        if (ev.session_id && !sessionID) setSessionID(ev.session_id)
      },
      () => setRunning(false),
    )
    setWsRef(ws)

    ws.onopen = () => {
      ws.send(JSON.stringify({
        intent,
        mode,
        hitl_mode:  hitlMode,
        project_id: projectID || undefined,
        title:      title || undefined,
      }))
    }
  }

  const cancel = () => {
    wsRef?.close()
    setRunning(false)
  }

  const copyLog = () => {
    const text = events.map(e => `[${e.type.toUpperCase()}] ${e.content || e.data || ''}`).join('\n\n')
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const clearLog = () => {
    setEvents([])
    setSessionID(null)
  }

  const eventColor = (type) => {
    if (type === 'tool_result') return 'var(--success)'
    if (type === 'final') return 'var(--accent)'
    if (type === 'error') return 'var(--danger)'
    if (type === 'tool_call') return 'var(--warn)'
    return 'var(--muted)'
  }

  const renderEvent = (ev, i) => {
    let content = ev.content || ''
    if (ev.data) {
      try { const d = JSON.parse(ev.data); content = JSON.stringify(d, null, 2) } catch { content = ev.data }
    }
    return (
      <div key={i} style={{ marginBottom: 8, padding: '8px 12px', background: 'var(--surface2)', borderRadius: 4, borderLeft: `3px solid ${eventColor(ev.type)}` }}>
        <div style={{ fontSize: 11, color: eventColor(ev.type), marginBottom: 2, fontWeight: 600, textTransform: 'uppercase' }}>{ev.type}</div>
        <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: 'monospace', fontSize: 12, color: 'var(--text)' }}>{content}</pre>
      </div>
    )
  }

  const filteredTools = tools.filter(t => {
    if (!catalogSearch) return true
    const q = catalogSearch.toLowerCase()
    return t.name.toLowerCase().includes(q) || (t.description && t.description.toLowerCase().includes(q))
  })

  const filteredSkills = skills.filter(s => {
    if (!catalogSearch) return true
    const q = catalogSearch.toLowerCase()
    return s.name.toLowerCase().includes(q) ||
      (s.dir_name && s.dir_name.toLowerCase().includes(q)) ||
      (s.description && s.description.toLowerCase().includes(q))
  })

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>Agent Operator Console</h1>
          <p style={{ color: 'var(--muted)', fontSize: 13, margin: '4px 0 0' }}>
            Autonomous and supervised security agent runs with real-time streaming telemetry.
          </p>
        </div>
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        {/* Row 1: Intent */}
        <div style={{ marginBottom: 12 }}>
          <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Security Intent *</label>
          <input
            type="text"
            value={intent}
            onChange={e => setIntent(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && run()}
            placeholder="e.g. Enumerate subdomains for example.com and probe live web services"
            style={{ width: '100%' }}
          />
        </div>

        {/* Row 2: Project / Title / Mode / HITL */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr auto auto', gap: 12, marginBottom: 12 }}>
          <div>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Project (optional)</label>
            <select value={projectID} onChange={e => setProjectID(e.target.value)}>
              <option value="">— No project —</option>
              {projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </div>
          <div>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Session title (optional)</label>
            <input
              type="text"
              value={title}
              onChange={e => setTitle(e.target.value)}
              placeholder="Auto-generated from intent"
            />
          </div>
          <div>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Mode</label>
            <select value={mode} onChange={e => setMode(e.target.value)} style={{ width: 140 }}>
              <option value="single">Single Agent</option>
              <option value="deep">DeepAgent (Multi)</option>
              <option value="plan_execute">Plan-Execute</option>
              <option value="supervisor">Supervisor</option>
            </select>
          </div>
          <div>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>HITL Safety</label>
            <select value={hitlMode} onChange={e => setHitlMode(e.target.value)} style={{ width: 150 }}>
              <option value="auto">Auto (Safe Only)</option>
              <option value="require_approval">Require Approval</option>
            </select>
          </div>
        </div>

        <div className="flex gap-8 items-center">
          <button className="primary" onClick={run} disabled={running || !intent.trim()}>
            {running ? '⏳ Executing…' : '▶ Run Agent'}
          </button>
          {running && <button className="danger" onClick={cancel}>■ Stop</button>}
          {sessionID && !running && (
            <Link to={`/sessions/${sessionID}`} style={{ fontSize: 12, color: 'var(--accent)', marginLeft: 8, fontWeight: 600 }}>
              View full session replay →
            </Link>
          )}
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '240px 1fr', gap: 16 }}>
        {/* Capability sidebar with Tools / Skills tabs */}
        <div className="card" style={{ display: 'flex', flexDirection: 'column', maxHeight: 560 }}>
          <div style={{ display: 'flex', borderBottom: '1px solid var(--border)', marginBottom: 8, paddingBottom: 6, gap: 4 }}>
            <button
              onClick={() => setSidebarTab('tools')}
              style={{
                flex: 1,
                fontSize: 12,
                fontWeight: sidebarTab === 'tools' ? 600 : 400,
                padding: '4px 6px',
                background: sidebarTab === 'tools' ? 'var(--surface2)' : 'transparent',
                color: sidebarTab === 'tools' ? 'var(--accent)' : 'var(--muted)',
                borderRadius: 4
              }}
            >
              Tools ({filteredTools.length})
            </button>
            <button
              onClick={() => setSidebarTab('skills')}
              style={{
                flex: 1,
                fontSize: 12,
                fontWeight: sidebarTab === 'skills' ? 600 : 400,
                padding: '4px 6px',
                background: sidebarTab === 'skills' ? 'var(--surface2)' : 'transparent',
                color: sidebarTab === 'skills' ? '#38bdf8' : 'var(--muted)',
                borderRadius: 4
              }}
            >
              Skills ({filteredSkills.length})
            </button>
          </div>

          <input
            type="text"
            placeholder={sidebarTab === 'tools' ? 'Filter tools...' : 'Filter skills...'}
            value={catalogSearch}
            onChange={e => setCatalogSearch(e.target.value)}
            style={{ fontSize: 11, padding: '4px 8px', marginBottom: 8 }}
          />

          <div style={{ flex: 1, overflowY: 'auto' }}>
            {sidebarTab === 'tools' ? (
              filteredTools.map(t => (
                <div key={t.name} style={{ marginBottom: 6, padding: '6px 8px', background: 'var(--surface2)', borderRadius: 4 }}>
                  <div style={{ fontWeight: 600, fontSize: 12, color: 'var(--accent)' }}>{t.name}</div>
                  <div style={{ color: 'var(--muted)', fontSize: 11, marginTop: 2 }}>{t.description?.slice(0, 60)}…</div>
                </div>
              ))
            ) : (
              filteredSkills.map(s => (
                <div key={s.id} style={{ marginBottom: 6, padding: '6px 8px', background: 'var(--surface2)', borderRadius: 4 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div style={{ fontWeight: 600, fontSize: 12, color: '#38bdf8' }}>{s.name}</div>
                    <button
                      onClick={() => {
                        setIntent(`Please activate and execute the "${s.name}" skill. Perform thorough assessment following its guidelines.`)
                        if (mode === 'single') setMode('deep')
                      }}
                      style={{ fontSize: 10, padding: '2px 6px', color: 'var(--accent)' }}
                      title="Insert trigger prompt into intent"
                    >
                      + Use
                    </button>
                  </div>
                  <div style={{ color: 'var(--muted)', fontSize: 11, marginTop: 2 }}>{s.description?.slice(0, 65)}…</div>
                </div>
              ))
            )}

            {sidebarTab === 'tools' && filteredTools.length === 0 && (
              <div style={{ color: 'var(--muted)', fontSize: 11, textAlign: 'center', padding: 12 }}>No matching tools</div>
            )}
            {sidebarTab === 'skills' && filteredSkills.length === 0 && (
              <div style={{ color: 'var(--muted)', fontSize: 11, textAlign: 'center', padding: 12 }}>No matching skills</div>
            )}
          </div>
        </div>

        {/* Execution stream pane */}
        <div className="card" style={{ padding: 0, display: 'flex', flexDirection: 'column', height: 560 }}>
          <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ fontWeight: 600, fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
              Execution Stream
              {running && <span style={{ color: 'var(--warn)', fontSize: 11 }}>● live</span>}
            </span>
            <div style={{ display: 'flex', gap: 6 }}>
              {events.length > 0 && (
                <>
                  <button onClick={copyLog} style={{ fontSize: 11, padding: '2px 8px' }}>
                    {copied ? '✓ Copied' : 'Copy'}
                  </button>
                  <button onClick={clearLog} style={{ fontSize: 11, padding: '2px 8px' }}>
                    Clear
                  </button>
                </>
              )}
            </div>
          </div>
          <div ref={logRef} style={{ flex: 1, overflowY: 'auto', padding: 14 }}>
            {events.length === 0 && (
              <div style={{ color: 'var(--muted)', fontSize: 13, textAlign: 'center', marginTop: 100 }}>
                Enter an intent above and click <strong>Run Agent</strong> to launch an execution session.
              </div>
            )}
            {events.map(renderEvent)}
          </div>
        </div>
      </div>
    </div>
  )
}
