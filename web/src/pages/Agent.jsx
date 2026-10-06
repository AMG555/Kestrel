import { useState, useEffect, useRef } from 'react'
import { api, connectAgentStream } from '../api.js'

export default function AgentPage() {
  const [tools, setTools]       = useState([])
  const [projects, setProjects] = useState([])
  const [intent, setIntent]     = useState('')
  const [projectID, setProjectID] = useState('')
  const [title, setTitle]       = useState('')
  const [mode, setMode]         = useState('single')
  const [hitlMode, setHitlMode] = useState('auto')
  const [events, setEvents]     = useState([])
  const [running, setRunning]   = useState(false)
  const [wsRef, setWsRef]       = useState(null)
  const [sessionID, setSessionID] = useState(null)
  const logRef = useRef(null)

  useEffect(() => {
    api.listTools().then(d => setTools(d.tools || []))
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

  return (
    <div>
      <h1 style={{ fontSize: 20, fontWeight: 700, marginBottom: 16 }}>Agent</h1>

      <div className="card" style={{ marginBottom: 16 }}>
        {/* Row 1: Intent */}
        <div style={{ marginBottom: 12 }}>
          <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Intent *</label>
          <input
            type="text"
            value={intent}
            onChange={e => setIntent(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && run()}
            placeholder="e.g. Enumerate subdomains for example.com"
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
            <select value={mode} onChange={e => setMode(e.target.value)} style={{ width: 130 }}>
              <option value="single">Single</option>
              <option value="plan_execute">Plan-Execute</option>
              <option value="supervisor">Supervisor</option>
            </select>
          </div>
          <div>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>HITL</label>
            <select value={hitlMode} onChange={e => setHitlMode(e.target.value)} style={{ width: 148 }}>
              <option value="auto">Auto</option>
              <option value="require_approval">Require Approval</option>
            </select>
          </div>
        </div>

        <div className="flex gap-8">
          <button className="primary" onClick={run} disabled={running || !intent.trim()}>
            {running ? '⏳ Running…' : '▶ Run'}
          </button>
          {running && <button className="danger" onClick={cancel}>■ Cancel</button>}
          {sessionID && !running && (
            <a href={`/sessions/${sessionID}`} style={{ fontSize: 12, color: 'var(--accent)', alignSelf: 'center', marginLeft: 8 }}>
              View session →
            </a>
          )}
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '200px 1fr', gap: 16 }}>
        <div className="card">
          <div style={{ fontWeight: 600, marginBottom: 10, fontSize: 13 }}>Available Tools</div>
          {tools.map(t => (
            <div key={t.name} style={{ marginBottom: 8, padding: '6px 8px', background: 'var(--surface2)', borderRadius: 4 }}>
              <div style={{ fontWeight: 600, fontSize: 12 }}>{t.name}</div>
              <div style={{ color: 'var(--muted)', fontSize: 11, marginTop: 2 }}>{t.description?.slice(0, 60)}…</div>
            </div>
          ))}
        </div>

        <div className="card" style={{ padding: 0 }}>
          <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--border)', fontWeight: 600, fontSize: 13 }}>
            Execution Stream {running && <span style={{ color: 'var(--warn)', fontSize: 11 }}>● live</span>}
          </div>
          <div ref={logRef} style={{ height: 480, overflowY: 'auto', padding: 14 }}>
            {events.length === 0 && (
              <div style={{ color: 'var(--muted)', fontSize: 13 }}>
                Enter an intent above and click Run to start an agent session.
              </div>
            )}
            {events.map(renderEvent)}
          </div>
        </div>
      </div>
    </div>
  )
}
