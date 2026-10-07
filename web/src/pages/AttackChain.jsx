import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../api.js'

const NODE_COLORS = {
  asset: '#3b82d4', finding: '#ef4444', technique: '#f59e0b',
  pivot: '#8b5cf6', objective: '#22c55e',
}

function riskColor(score) {
  if (score >= 8) return '#ef4444'
  if (score >= 5) return '#f59e0b'
  return '#22c55e'
}

export default function AttackChainPage() {
  const { id } = useParams()
  const [project, setProject]   = useState(null)
  const [nodes, setNodes]       = useState([])
  const [edges, setEdges]       = useState([])
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState('')
  const [addNodeForm, setAddNodeForm] = useState(null)
  const [addEdgeForm, setAddEdgeForm] = useState(null)
  const [saving, setSaving]     = useState(false)
  const [promoting, setPromoting] = useState(false)
  const [promoteNotice, setPromoteNotice] = useState(null)

  const handleAutoPromote = async () => {
    const sid = window.prompt("Enter Agent Session ID to promote findings from (or leave empty for all project sessions):")
    if (sid === null) return
    setPromoting(true)
    setPromoteNotice(null)
    try {
      const res = await api.promoteAttackChain(id, sid.trim())
      setPromoteNotice(`Auto-promoted successfully: ${res.nodes_created} nodes, ${res.edges_created} edges, ${res.facts_created} facts created!`)
      load()
    } catch (err) {
      setError(`Auto-promote failed: ${err.message}`)
    } finally {
      setPromoting(false)
    }
  }

  const load = () => {
    setLoading(true)
    Promise.all([api.getProject(id), api.getAttackChain(id)])
      .then(([pd, cd]) => {
        setProject(pd.project)
        setNodes(cd.nodes || [])
        setEdges(cd.edges || [])
        setLoading(false)
      })
      .catch(e => { setError(e.message); setLoading(false) })
  }

  useEffect(() => { load() }, [id])

  const handleAddNode = async (e) => {
    e.preventDefault()
    setSaving(true)
    try {
      await api.addChainNode(id, addNodeForm)
      setAddNodeForm(null)
      load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleAddEdge = async (e) => {
    e.preventDefault()
    setSaving(true)
    try {
      await api.addChainEdge(id, { ...addEdgeForm, edge_type: addEdgeForm.edge_type || 'leads_to' })
      setAddEdgeForm(null)
      load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  // Simple layered layout for SVG rendering
  const nodeMap = {}
  nodes.forEach(n => { nodeMap[n.id] = n })

  const inDegree = {}
  nodes.forEach(n => { inDegree[n.id] = 0 })
  edges.forEach(e => { inDegree[e.target_node_id] = (inDegree[e.target_node_id] || 0) + 1 })

  const layers = []
  const assigned = {}
  let queue = nodes.filter(n => inDegree[n.id] === 0).map(n => n.id)
  while (queue.length > 0) {
    layers.push([...queue])
    queue.forEach(nid => { assigned[nid] = true })
    const next = []
    edges.forEach(e => {
      if (assigned[e.source_node_id] && !assigned[e.target_node_id]) {
        if (!next.includes(e.target_node_id)) next.push(e.target_node_id)
      }
    })
    queue = next
  }
  nodes.forEach(n => { if (!assigned[n.id]) { layers.push([n.id]) } })

  const positions = {}
  const NW = 140, NH = 44, HGAP = 160, VGAP = 80, PAD = 40
  layers.forEach((layer, li) => {
    const y = PAD + li * (NH + VGAP)
    layer.forEach((nid, ni) => {
      const x = PAD + ni * (NW + HGAP)
      positions[nid] = { x, y }
    })
  })

  const svgW = Math.max(...Object.values(positions).map(p => p.x + NW + PAD), 400)
  const svgH = Math.max(...Object.values(positions).map(p => p.y + NH + PAD), 200)

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 20 }}>
        <Link to="/projects" style={{ color: 'var(--muted)', fontSize: 13, textDecoration: 'none' }}>← Projects</Link>
        <h2 style={{ margin: 0 }}>Attack Chain — {project?.name || '…'}</h2>
      </div>

      {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}
      {promoteNotice && (
        <div style={{ background: '#064e3b33', border: '1px solid #059669', color: '#6ee7b7', padding: '10px 14px', borderRadius: '6px', marginBottom: 12, fontSize: '13px' }}>
          {promoteNotice}
        </div>
      )}

      {/* Controls */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        <button onClick={() => setAddNodeForm({ node_type: 'asset', node_name: '', risk_score: 0 })}>+ Add Node</button>
        {nodes.length >= 2 && (
          <button onClick={() => setAddEdgeForm({ source_node_id: '', target_node_id: '', edge_type: 'leads_to' })}>+ Add Edge</button>
        )}
        <button 
          onClick={handleAutoPromote} 
          disabled={promoting}
          style={{ background: '#7c3aed', color: '#ffffff', border: 'none', borderRadius: '4px', padding: '6px 12px', cursor: promoting ? 'not-allowed' : 'pointer', fontWeight: 500 }}
        >
          {promoting ? 'Promoting...' : '⚡ Auto-Promote Session'}
        </button>
      </div>

      {addNodeForm && (
        <form onSubmit={handleAddNode} style={{
          background: 'var(--surface)', border: '1px solid var(--border)',
          borderRadius: 8, padding: 14, marginBottom: 16,
          display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10,
        }}>
          <div>
            <label style={{ fontSize: 11, color: 'var(--muted)' }}>Type *</label>
            <select value={addNodeForm.node_type}
              onChange={e => setAddNodeForm(f => ({ ...f, node_type: e.target.value }))}
              style={{ width: '100%', marginTop: 4 }}>
              {['asset', 'finding', 'technique', 'pivot', 'objective'].map(t => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </div>
          <div style={{ gridColumn: 'span 2' }}>
            <label style={{ fontSize: 11, color: 'var(--muted)' }}>Name *</label>
            <input value={addNodeForm.node_name} required
              onChange={e => setAddNodeForm(f => ({ ...f, node_name: e.target.value }))}
              style={{ width: '100%', marginTop: 4 }} placeholder="Node name" />
          </div>
          <div>
            <label style={{ fontSize: 11, color: 'var(--muted)' }}>Risk Score (0-10)</label>
            <input type="number" min={0} max={10} value={addNodeForm.risk_score}
              onChange={e => setAddNodeForm(f => ({ ...f, risk_score: parseInt(e.target.value, 10) || 0 }))}
              style={{ width: '100%', marginTop: 4 }} />
          </div>
          <div style={{ gridColumn: 'span 4', display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
            <button type="button" onClick={() => setAddNodeForm(null)}>Cancel</button>
            <button type="submit" disabled={saving} className="primary">{saving ? 'Adding…' : 'Add Node'}</button>
          </div>
        </form>
      )}

      {addEdgeForm && (
        <form onSubmit={handleAddEdge} style={{
          background: 'var(--surface)', border: '1px solid var(--border)',
          borderRadius: 8, padding: 14, marginBottom: 16,
          display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10,
        }}>
          <div>
            <label style={{ fontSize: 11, color: 'var(--muted)' }}>Source Node *</label>
            <select value={addEdgeForm.source_node_id} required
              onChange={e => setAddEdgeForm(f => ({ ...f, source_node_id: e.target.value }))}
              style={{ width: '100%', marginTop: 4 }}>
              <option value="">-- select --</option>
              {nodes.map(n => <option key={n.id} value={n.id}>{n.node_name} ({n.node_type})</option>)}
            </select>
          </div>
          <div>
            <label style={{ fontSize: 11, color: 'var(--muted)' }}>Target Node *</label>
            <select value={addEdgeForm.target_node_id} required
              onChange={e => setAddEdgeForm(f => ({ ...f, target_node_id: e.target.value }))}
              style={{ width: '100%', marginTop: 4 }}>
              <option value="">-- select --</option>
              {nodes.map(n => <option key={n.id} value={n.id}>{n.node_name} ({n.node_type})</option>)}
            </select>
          </div>
          <div>
            <label style={{ fontSize: 11, color: 'var(--muted)' }}>Edge Type</label>
            <select value={addEdgeForm.edge_type}
              onChange={e => setAddEdgeForm(f => ({ ...f, edge_type: e.target.value }))}
              style={{ width: '100%', marginTop: 4 }}>
              {['leads_to', 'depends_on', 'enables', 'mitigates'].map(t => <option key={t} value={t}>{t}</option>)}
            </select>
          </div>
          <div style={{ gridColumn: 'span 3', display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
            <button type="button" onClick={() => setAddEdgeForm(null)}>Cancel</button>
            <button type="submit" disabled={saving} className="primary">{saving ? 'Adding…' : 'Add Edge'}</button>
          </div>
        </form>
      )}

      {loading ? (
        <div style={{ color: 'var(--muted)', padding: 24 }}>Loading…</div>
      ) : nodes.length === 0 ? (
        <div style={{ color: 'var(--muted)', padding: 24, textAlign: 'center' }}>
          No nodes yet. Add the first node to start the attack chain.
        </div>
      ) : (
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, overflowX: 'auto', padding: 12 }}>
          <svg width={svgW} height={svgH}>
            <defs>
              <marker id="arrowhead" markerWidth={10} markerHeight={7} refX={9} refY={3.5} orient="auto">
                <polygon points="0 0, 10 3.5, 0 7" fill="var(--muted)" />
              </marker>
            </defs>

            {edges.map(e => {
              const sp = positions[e.source_node_id]
              const tp = positions[e.target_node_id]
              if (!sp || !tp) return null
              const x1 = sp.x + NW / 2, y1 = sp.y + NH
              const x2 = tp.x + NW / 2, y2 = tp.y
              return (
                <g key={e.id}>
                  <line x1={x1} y1={y1} x2={x2} y2={y2}
                    stroke="var(--border)" strokeWidth={1.5} markerEnd="url(#arrowhead)" />
                  <text x={(x1 + x2) / 2 + 4} y={(y1 + y2) / 2}
                    fontSize={9} fill="var(--muted)">{e.edge_type}</text>
                </g>
              )
            })}

            {nodes.map(n => {
              const pos = positions[n.id]
              if (!pos) return null
              const color = NODE_COLORS[n.node_type] || '#94a3b8'
              return (
                <g key={n.id}>
                  <rect x={pos.x} y={pos.y} width={NW} height={NH} rx={6}
                    fill={color + '22'} stroke={color} strokeWidth={1.5} />
                  <text x={pos.x + NW / 2} y={pos.y + 14}
                    fontSize={9} fontWeight={700} fill={color} textAnchor="middle">
                    {n.node_type.toUpperCase()}
                  </text>
                  <text x={pos.x + NW / 2} y={pos.y + 28}
                    fontSize={11} fill="var(--text)" textAnchor="middle">
                    {n.node_name.length > 16 ? n.node_name.slice(0, 15) + '…' : n.node_name}
                  </text>
                  {n.risk_score > 0 && (
                    <text x={pos.x + NW - 6} y={pos.y + 14}
                      fontSize={9} fill={riskColor(n.risk_score)} textAnchor="end" fontWeight={700}>
                      {n.risk_score}
                    </text>
                  )}
                </g>
              )
            })}
          </svg>
        </div>
      )}

      {/* Node list table */}
      {nodes.length > 0 && (
        <div style={{ marginTop: 20 }}>
          <h3 style={{ fontSize: 14, marginBottom: 10 }}>Nodes ({nodes.length})</h3>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ background: 'var(--surface)', borderBottom: '1px solid var(--border)' }}>
                {['Type', 'Name', 'Risk', 'Session'].map(h => (
                  <th key={h} style={{ padding: '6px 10px', textAlign: 'left', color: 'var(--muted)', fontWeight: 600 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {nodes.map(n => (
                <tr key={n.id} style={{ borderBottom: '1px solid var(--border)' }}>
                  <td style={{ padding: '6px 10px' }}>
                    <span style={{ color: NODE_COLORS[n.node_type] || '#94a3b8', fontWeight: 600 }}>{n.node_type}</span>
                  </td>
                  <td style={{ padding: '6px 10px' }}>{n.node_name}</td>
                  <td style={{ padding: '6px 10px', color: riskColor(n.risk_score) }}>{n.risk_score || '—'}</td>
                  <td style={{ padding: '6px 10px', color: 'var(--muted)', fontFamily: 'monospace', fontSize: 11 }}>
                    {n.session_id ? n.session_id.slice(0, 16) + '…' : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
