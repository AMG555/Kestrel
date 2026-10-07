import React, { useState, useEffect } from 'react'
import { api } from '../api'

const CATEGORIES = [
  { id: '', label: 'All Categories' },
  { id: 'reconnaissance', label: 'Reconnaissance' },
  { id: 'scanning', label: 'Port & Service Scanning' },
  { id: 'content_discovery', label: 'Content & Directory Fuzzing' },
  { id: 'vulnerability_audit', label: 'Vulnerability Audit' },
  { id: 'cloud_container', label: 'Cloud & Container' },
  { id: 'binary_reversing', label: 'Binary & Reverse Engineering' },
  { id: 'credential_assessment', label: 'Credential Assessment' },
  { id: 'general_security', label: 'General Utilities' },
]

export default function Tools() {
  const [tools, setTools] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState('')
  const [selectedTool, setSelectedTool] = useState(null)
  const [executing, setExecuting] = useState(false)
  const [execResult, setExecResult] = useState(null)
  const [execArgs, setExecArgs] = useState({})

  useEffect(() => {
    loadTools()
  }, [category])

  async function loadTools() {
    setLoading(true)
    setError(null)
    try {
      const q = {}
      if (category) q.category = category
      const data = await api.listToolCatalog(q)
      setTools(data.tools || [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const filtered = tools.filter(t => {
    if (!search) return true
    const s = search.toLowerCase()
    return t.name.toLowerCase().includes(s) ||
      (t.command && t.command.toLowerCase().includes(s)) ||
      (t.short_description && t.short_description.toLowerCase().includes(s)) ||
      (t.description && t.description.toLowerCase().includes(s))
  })

  function openDetail(tool) {
    setSelectedTool(tool)
    setExecResult(null)
    const initialArgs = {}
    if (tool.parameters) {
      tool.parameters.forEach(p => {
        if (p.default !== undefined) initialArgs[p.name] = p.default
      })
    }
    setExecArgs(initialArgs)
  }

  async function runQuickTest() {
    if (!selectedTool) return
    setExecuting(true)
    setExecResult(null)
    try {
      const res = await api.executeTool(selectedTool.name, execArgs)
      setExecResult(res)
    } catch (err) {
      setExecResult({ error: err.message })
    } finally {
      setExecuting(false)
    }
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1400px', margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '24px' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '26px', fontWeight: 700, display: 'flex', alignItems: 'center', gap: '10px' }}>
            <span>🧰</span> Security Tool Recipes Catalog
          </h1>
          <p style={{ margin: '6px 0 0', color: '#94a3b8', fontSize: '14px' }}>
            Curated library of security tool recipes with defined flag parameters, input schemas, and role scoping.
          </p>
        </div>
        <button
          onClick={loadTools}
          style={{
            background: '#1e293b',
            color: '#e2e8f0',
            border: '1px solid #334155',
            padding: '8px 16px',
            borderRadius: '6px',
            cursor: 'pointer',
            fontSize: '13px',
            fontWeight: 500
          }}
        >
          ↻ Refresh
        </button>
      </div>

      {/* Category Pills & Search */}
      <div style={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: '10px', padding: '16px', marginBottom: '24px' }}>
        <div style={{ marginBottom: '14px' }}>
          <input
            type="text"
            placeholder="Search tool recipes (e.g. nmap, nuclei, dirsearch, sqlmap, ffuf)..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            style={{
              width: '100%',
              boxSizing: 'border-box',
              background: '#1e293b',
              border: '1px solid #334155',
              borderRadius: '6px',
              padding: '10px 14px',
              color: '#f8fafc',
              fontSize: '14px'
            }}
          />
        </div>

        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
          {CATEGORIES.map(cat => (
            <button
              key={cat.id}
              onClick={() => setCategory(cat.id)}
              style={{
                background: category === cat.id ? '#2563eb' : '#1e293b',
                color: category === cat.id ? '#ffffff' : '#94a3b8',
                border: '1px solid',
                borderColor: category === cat.id ? '#3b82f6' : '#334155',
                padding: '6px 12px',
                borderRadius: '6px',
                cursor: 'pointer',
                fontSize: '12px',
                fontWeight: 500,
                transition: 'all 0.15s ease'
              }}
            >
              {cat.label}
            </button>
          ))}
        </div>
      </div>

      {/* Stats Counter */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px', color: '#64748b', fontSize: '13px' }}>
        <span>Showing <strong>{filtered.length}</strong> tool recipes</span>
        <span>Curated Declarative Schemas</span>
      </div>

      {/* Error state */}
      {error && (
        <div style={{ background: '#7f1d1d33', border: '1px solid #dc2626', color: '#fca5a5', padding: '12px', borderRadius: '8px', marginBottom: '20px' }}>
          {error}
        </div>
      )}

      {/* Tools Grid */}
      {loading ? (
        <div style={{ padding: '60px', textAlign: 'center', color: '#64748b' }}>Loading tool recipes...</div>
      ) : filtered.length === 0 ? (
        <div style={{ padding: '60px', textAlign: 'center', color: '#64748b', background: '#0f172a', borderRadius: '10px', border: '1px dashed #334155' }}>
          No tools matched your criteria.
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(360px, 1fr))', gap: '16px' }}>
          {filtered.map(tool => (
            <div
              key={tool.name}
              onClick={() => openDetail(tool)}
              style={{
                background: '#0f172a',
                border: '1px solid #1e293b',
                borderRadius: '10px',
                padding: '18px',
                cursor: 'pointer',
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'space-between',
                transition: 'transform 0.15s ease, border-color 0.15s ease'
              }}
              onMouseEnter={e => {
                e.currentTarget.style.borderColor = '#10b981'
                e.currentTarget.style.transform = 'translateY(-2px)'
              }}
              onMouseLeave={e => {
                e.currentTarget.style.borderColor = '#1e293b'
                e.currentTarget.style.transform = 'none'
              }}
            >
              <div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '8px' }}>
                  <h3 style={{ margin: 0, fontSize: '16px', fontWeight: 600, color: '#f1f5f9' }}>
                    {tool.name}
                  </h3>
                  <span style={{ fontSize: '11px', background: '#10b98122', color: '#10b981', padding: '2px 8px', borderRadius: '4px', textTransform: 'capitalize' }}>
                    {tool.category ? tool.category.replace('_', ' ') : 'General'}
                  </span>
                </div>
                <div style={{ marginBottom: '10px' }}>
                  <code style={{ fontSize: '12px', background: '#1e293b', color: '#38bdf8', padding: '2px 6px', borderRadius: '4px' }}>
                    {tool.command} {tool.args ? tool.args.join(' ') : ''}
                  </code>
                </div>
                <p style={{ margin: '0 0 14px', fontSize: '13px', color: '#94a3b8', lineHeight: '1.5', minHeight: '38px' }}>
                  {tool.short_description || 'No description provided.'}
                </p>
              </div>

              <div>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', color: '#64748b', borderTop: '1px solid #1e293b', paddingTop: '10px' }}>
                  <span>{tool.parameters ? tool.parameters.length : 0} parameters</span>
                  <span style={{ color: '#10b981', fontWeight: 500 }}>View Schema →</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Tool Detail Modal */}
      {selectedTool && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0,0,0,0.75)',
          backdropFilter: 'blur(4px)',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          zIndex: 1000,
          padding: '20px'
        }}>
          <div style={{
            background: '#0f172a',
            border: '1px solid #334155',
            borderRadius: '12px',
            width: '100%',
            maxWidth: '920px',
            maxHeight: '90vh',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            boxShadow: '0 25px 50px -12px rgba(0,0,0,0.5)'
          }}>
            {/* Header */}
            <div style={{ padding: '18px 24px', borderBottom: '1px solid #1e293b', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <h2 style={{ margin: 0, fontSize: '18px', color: '#f8fafc', fontWeight: 600 }}>
                  Recipe: {selectedTool.name}
                </h2>
                <span style={{ fontSize: '12px', color: '#10b981', textTransform: 'capitalize' }}>
                  Category: {selectedTool.category ? selectedTool.category.replace('_', ' ') : 'General'}
                </span>
              </div>
              <button
                onClick={() => setSelectedTool(null)}
                style={{ background: 'transparent', border: 'none', color: '#94a3b8', fontSize: '20px', cursor: 'pointer' }}
              >
                ✕
              </button>
            </div>

            {/* Body */}
            <div style={{ padding: '24px', overflowY: 'auto', flex: 1 }}>
              {/* Command Banner */}
              <div style={{ background: '#020617', border: '1px solid #1e293b', borderRadius: '8px', padding: '14px 18px', marginBottom: '20px' }}>
                <div style={{ fontSize: '12px', color: '#64748b', marginBottom: '4px', textTransform: 'uppercase', fontWeight: 600 }}>Command Execution Template</div>
                <code style={{ fontSize: '14px', color: '#38bdf8' }}>
                  {selectedTool.command} {selectedTool.args ? selectedTool.args.join(' ') : ''}
                </code>
              </div>

              {/* Description */}
              <div style={{ marginBottom: '20px' }}>
                <h4 style={{ margin: '0 0 8px', fontSize: '13px', textTransform: 'uppercase', color: '#64748b' }}>Description</h4>
                <div style={{ background: '#1e293b', borderRadius: '8px', padding: '14px', color: '#e2e8f0', fontSize: '13px', lineHeight: '1.6', whiteSpace: 'pre-wrap' }}>
                  {selectedTool.description || selectedTool.short_description || 'No description available.'}
                </div>
              </div>

              {/* Parameter Table */}
              <div style={{ marginBottom: '20px' }}>
                <h4 style={{ margin: '0 0 10px', fontSize: '13px', textTransform: 'uppercase', color: '#64748b' }}>
                  Parameter Specification ({selectedTool.parameters ? selectedTool.parameters.length : 0})
                </h4>
                {selectedTool.parameters && selectedTool.parameters.length > 0 ? (
                  <div style={{ overflowX: 'auto', border: '1px solid #1e293b', borderRadius: '8px' }}>
                    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '13px', textAlign: 'left' }}>
                      <thead>
                        <tr style={{ background: '#1e293b', color: '#94a3b8' }}>
                          <th style={{ padding: '10px 14px' }}>Name</th>
                          <th style={{ padding: '10px 14px' }}>Flag / Format</th>
                          <th style={{ padding: '10px 14px' }}>Type</th>
                          <th style={{ padding: '10px 14px' }}>Required</th>
                          <th style={{ padding: '10px 14px' }}>Description</th>
                        </tr>
                      </thead>
                      <tbody>
                        {selectedTool.parameters.map((p, idx) => (
                          <tr key={p.name} style={{ borderTop: '1px solid #1e293b', background: idx % 2 === 0 ? 'transparent' : '#0f172a88' }}>
                            <td style={{ padding: '10px 14px', color: '#38bdf8', fontWeight: 500 }}>{p.name}</td>
                            <td style={{ padding: '10px 14px', fontFamily: 'monospace', color: '#e2e8f0' }}>{p.flag || p.format || '-'}</td>
                            <td style={{ padding: '10px 14px', color: '#cbd5e1' }}>{p.type}</td>
                            <td style={{ padding: '10px 14px' }}>
                              {p.required ? (
                                <span style={{ color: '#ef4444', fontWeight: 600 }}>Yes</span>
                              ) : (
                                <span style={{ color: '#64748b' }}>No</span>
                              )}
                            </td>
                            <td style={{ padding: '10px 14px', color: '#94a3b8' }}>{p.description}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <p style={{ color: '#64748b', fontSize: '13px' }}>No parameters defined for this tool.</p>
                )}
              </div>

              {/* Quick Test Console */}
              <div style={{ background: '#020617', border: '1px solid #334155', borderRadius: '8px', padding: '16px', marginTop: '20px' }}>
                <h4 style={{ margin: '0 0 12px', fontSize: '13px', color: '#f8fafc', fontWeight: 600 }}>Quick Test Execution</h4>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: '12px', marginBottom: '14px' }}>
                  {(selectedTool.parameters || []).map(p => (
                    <div key={p.name}>
                      <label style={{ display: 'block', fontSize: '12px', color: '#94a3b8', marginBottom: '4px' }}>
                        {p.name} {p.required && <span style={{ color: '#ef4444' }}>*</span>}
                      </label>
                      <input
                        type="text"
                        placeholder={p.description ? p.description.slice(0, 30) + '...' : ''}
                        value={execArgs[p.name] || ''}
                        onChange={e => setExecArgs({ ...execArgs, [p.name]: e.target.value })}
                        style={{
                          width: '100%',
                          boxSizing: 'border-box',
                          background: '#0f172a',
                          border: '1px solid #334155',
                          borderRadius: '4px',
                          padding: '6px 10px',
                          color: '#f8fafc',
                          fontSize: '12px'
                        }}
                      />
                    </div>
                  ))}
                </div>
                <button
                  onClick={runQuickTest}
                  disabled={executing}
                  style={{
                    background: '#10b981',
                    color: '#ffffff',
                    border: 'none',
                    padding: '8px 18px',
                    borderRadius: '6px',
                    cursor: executing ? 'not-allowed' : 'pointer',
                    fontWeight: 600,
                    fontSize: '13px'
                  }}
                >
                  {executing ? 'Executing...' : 'Run Tool Test ▶'}
                </button>

                {execResult && (
                  <div style={{ marginTop: '14px' }}>
                    <div style={{ fontSize: '12px', color: '#64748b', marginBottom: '4px' }}>Execution Output:</div>
                    <pre style={{
                      background: '#0a0f1d',
                      border: '1px solid #1e293b',
                      borderRadius: '6px',
                      padding: '12px',
                      color: execResult.error ? '#f87171' : '#38bdf8',
                      fontSize: '12px',
                      maxHeight: '200px',
                      overflowY: 'auto'
                    }}>
                      {execResult.error || JSON.stringify(execResult, null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            </div>

            {/* Footer */}
            <div style={{ padding: '14px 24px', borderTop: '1px solid #1e293b', display: 'flex', justifyContent: 'flex-end', background: '#0b1329' }}>
              <button
                onClick={() => setSelectedTool(null)}
                style={{
                  background: '#334155',
                  color: '#f8fafc',
                  border: 'none',
                  padding: '8px 18px',
                  borderRadius: '6px',
                  cursor: 'pointer',
                  fontWeight: 500,
                  fontSize: '13px'
                }}
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
