import { useState, useEffect, useRef } from 'react'
import { api } from '../api.js'

const DEFAULT_GRAPH = {
  nodes: [
    { id: 'start', type: 'agent', label: 'Recon Agent', config: { intent: 'Enumerate subdomains', mode: 'single' }, position: { x: 100, y: 80 } },
    { id: 'out', type: 'output', label: 'Output', config: { variables: ['start'] }, position: { x: 100, y: 220 } },
  ],
  edges: [
    { id: 'e1', source: 'start', target: 'out', label: '' },
  ],
}

const NODE_COLORS = { agent: '#3b82d4', tool: '#f59e0b', condition: '#8b5cf6', approval: '#ef4444', output: '#22c55e' }

function WorkflowCanvas({ graph }) {
  if (!graph || !graph.nodes) return null
  const nodes = graph.nodes
  const edges = graph.edges || []

  const nodeMap = {}
  nodes.forEach(n => { nodeMap[n.id] = n })

  const W = 160, H = 48, PAD_X = 80, PAD_Y = 48

  // Compute positions
  const posOf = (n) => ({
    x: (n.position?.x || 0) + PAD_X,
    y: (n.position?.y || 0) + PAD_Y,
  })

  const svgW = Math.max(...nodes.map(n => (n.position?.x || 0) + W + 2 * PAD_X), 400)
  const svgH = Math.max(...nodes.map(n => (n.position?.y || 0) + H + 2 * PAD_Y), 300)

  return (
    <svg width={svgW} height={svgH} style={{ overflow: 'visible' }}>
      {/* Edges */}
      {edges.map(e => {
        const s = nodeMap[e.source], t = nodeMap[e.target]
        if (!s || !t) return null
        const sp = posOf(s), tp = posOf(t)
        const x1 = sp.x + W / 2, y1 = sp.y + H
        const x2 = tp.x + W / 2, y2 = tp.y
        const my = (y1 + y2) / 2
        return (
          <g key={e.id}>
            <path d={`M${x1},${y1} C${x1},${my} ${x2},${my} ${x2},${y2}`}
              stroke="var(--border)" strokeWidth={2} fill="none" markerEnd="url(#arrow)" />
            {e.label && (
              <text x={(x1 + x2) / 2} y={my} fontSize={10} fill="var(--muted)" textAnchor="middle">{e.label}</text>
            )}
          </g>
        )
      })}

      {/* Nodes */}
      {nodes.map(n => {
        const { x, y } = posOf(n)
        const color = NODE_COLORS[n.type] || '#94a3b8'
        return (
          <g key={n.id}>
            <rect x={x} y={y} width={W} height={H} rx={6}
              fill={color + '22'} stroke={color} strokeWidth={1.5} />
            <text x={x + W / 2} y={y + 16} fontSize={10} fill={color} textAnchor="middle" fontWeight={600}>
              {n.type.toUpperCase()}
            </text>
            <text x={x + W / 2} y={y + 32} fontSize={11} fill="var(--text)" textAnchor="middle">
              {n.label || n.id}
            </text>
          </g>
        )
      })}

      <defs>
        <marker id="arrow" markerWidth={8} markerHeight={8} refX={6} refY={3} orient="auto">
          <path d="M0,0 L0,6 L8,3 z" fill="var(--muted)" />
        </marker>
      </defs>
    </svg>
  )
}

export default function WorkflowsPage() {
  const [workflows, setWorkflows]   = useState([])
  const [loading, setLoading]       = useState(true)
  const [error, setError]           = useState('')
  const [showForm, setShowForm]     = useState(false)
  const [form, setForm]             = useState({ name: '', description: '', graphStr: JSON.stringify(DEFAULT_GRAPH, null, 2) })
  const [graphErr, setGraphErr]     = useState('')
  const [saving, setSaving]         = useState(false)
  const [selected, setSelected]     = useState(null)
  const [runId, setRunId]           = useState(null)
  const [runStatus, setRunStatus]   = useState(null)
  const pollRef                     = useRef(null)

  const load = () => {
    setLoading(true)
    api.listWorkflows().then(d => { setWorkflows(d.workflows || []); setLoading(false) })
      .catch(e => { setError(e.message); setLoading(false) })
  }

  useEffect(() => { load() }, [])

  const handleCreate = async (e) => {
    e.preventDefault()
    let graph
    try { graph = JSON.parse(form.graphStr); setGraphErr('') } catch { setGraphErr('Invalid JSON'); return }
    setSaving(true)
    try {
      await api.createWorkflow({ name: form.name, description: form.description, graph })
      setShowForm(false)
      setForm({ name: '', description: '', graphStr: JSON.stringify(DEFAULT_GRAPH, null, 2) })
      load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleRun = async (id) => {
    setRunId(null)
    setRunStatus(null)
    const d = await api.runWorkflow(id, {}).catch(e => { setError(e.message); return null })
    if (!d) return
    setRunId(d.run_id)
    setRunStatus('running')
    pollRef.current = setInterval(async () => {
      const r = await api.getWorkflowRun(d.run_id).catch(() => null)
      if (r) {
        setRunStatus(r.status)
        if (r.status !== 'running') clearInterval(pollRef.current)
      }
    }, 3000)
  }

  const handleDelete = async (id) => {
    if (!confirm('Delete this workflow?')) return
    await api.deleteWorkflow(id).catch(e => setError(e.message))
    if (selected?.id === id) setSelected(null)
    load()
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h2 style={{ margin: 0 }}>Workflows</h2>
        <button onClick={() => setShowForm(v => !v)}>{showForm ? 'Cancel' : '+ New Workflow'}</button>
      </div>
      <p style={{ color: 'var(--muted)', fontSize: 13, marginTop: -12, marginBottom: 20 }}>
        Graph-based workflow definitions. Each node runs an agent, tool call, condition, or approval gate.
      </p>

      {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}

      {showForm && (
        <form onSubmit={handleCreate} style={{
          background: 'var(--surface)', border: '1px solid var(--border)',
          borderRadius: 8, padding: 16, marginBottom: 20, display: 'grid', gap: 10,
        }}>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
            <div>
              <label style={{ fontSize: 12, color: 'var(--muted)' }}>Name *</label>
              <input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                required style={{ width: '100%', marginTop: 4 }} />
            </div>
            <div>
              <label style={{ fontSize: 12, color: 'var(--muted)' }}>Description</label>
              <input value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
                style={{ width: '100%', marginTop: 4 }} />
            </div>
          </div>
          <div>
            <label style={{ fontSize: 12, color: 'var(--muted)' }}>Graph JSON</label>
            <textarea value={form.graphStr} onChange={e => setForm(f => ({ ...f, graphStr: e.target.value }))}
              rows={10} style={{ width: '100%', marginTop: 4, fontFamily: 'monospace', fontSize: 11 }} />
            {graphErr && <div style={{ color: 'var(--danger)', fontSize: 11, marginTop: 4 }}>{graphErr}</div>}
          </div>
          <div style={{ textAlign: 'right' }}>
            <button type="submit" disabled={saving} className="primary">{saving ? 'Creating…' : 'Create Workflow'}</button>
          </div>
        </form>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: selected ? '300px 1fr' : '1fr', gap: 16 }}>
        <div style={{ display: 'grid', gap: 10, alignContent: 'start' }}>
          {loading ? <div style={{ color: 'var(--muted)' }}>Loading…</div>
          : workflows.length === 0 ? <div style={{ color: 'var(--muted)', textAlign: 'center', padding: 24 }}>No workflows yet.</div>
          : workflows.map(w => (
            <div key={w.id} onClick={() => setSelected(w)}
              style={{
                background: 'var(--surface)', border: `1px solid ${selected?.id === w.id ? 'var(--accent)' : 'var(--border)'}`,
                borderRadius: 8, padding: '12px 14px', cursor: 'pointer',
              }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                <span style={{ fontWeight: 600, fontSize: 13 }}>{w.name}</span>
                <span style={{ fontSize: 11, color: w.enabled ? '#22c55e' : 'var(--muted)' }}>
                  {w.enabled ? 'enabled' : 'disabled'}
                </span>
              </div>
              {w.description && <div style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 8 }}>{w.description}</div>}
              <div style={{ display: 'flex', gap: 6 }}>
                <button onClick={e => { e.stopPropagation(); handleRun(w.id) }}
                  style={{ fontSize: 11, padding: '3px 10px' }} className="primary">▶ Run</button>
                <button onClick={e => { e.stopPropagation(); handleDelete(w.id) }}
                  style={{ fontSize: 11, padding: '3px 10px', color: 'var(--danger)' }}>Delete</button>
              </div>
            </div>
          ))}
        </div>

        {selected && (
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 16 }}>
            <h3 style={{ margin: '0 0 12px' }}>{selected.name}</h3>
            {runId && (
              <div style={{ marginBottom: 12, fontSize: 13 }}>
                Run <code style={{ fontSize: 11 }}>{runId}</code>
                — status: <strong>{runStatus}</strong>
              </div>
            )}
            <div style={{ overflowX: 'auto' }}>
              <WorkflowCanvas graph={(() => { try { return JSON.parse(selected.graph_json) } catch { return null } })()} />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
