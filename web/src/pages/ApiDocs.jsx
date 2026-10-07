import React, { useState, useEffect } from 'react'
import { api } from '../api'

const METHOD_COLORS = {
  get: { bg: 'rgba(56, 139, 253, 0.15)', text: '#58a6ff', border: '#58a6ff' },
  post: { bg: 'rgba(63, 185, 80, 0.15)', text: '#3fb950', border: '#3fb950' },
  patch: { bg: 'rgba(210, 153, 34, 0.15)', text: '#d29922', border: '#d29922' },
  put: { bg: 'rgba(210, 153, 34, 0.15)', text: '#d29922', border: '#d29922' },
  delete: { bg: 'rgba(248, 81, 73, 0.15)', text: '#f85149', border: '#f85149' },
}

export default function ApiDocs() {
  const [spec, setSpec] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState({})
  const [testResult, setTestResult] = useState({})
  const [testing, setTesting] = useState({})

  useEffect(() => {
    fetchSpec()
  }, [])

  async function fetchSpec() {
    setLoading(true)
    try {
      const data = await api.getOpenApiSpec()
      setSpec(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  function toggleExpand(key) {
    setExpanded(prev => ({ ...prev, [key]: !prev[key] }))
  }

  function copyCurl(method, path) {
    const token = localStorage.getItem('kestrel_token') || 'YOUR_API_TOKEN'
    const cmd = `curl -X ${method.toUpperCase()} "${window.location.origin}/api${path}" \\\n  -H "Authorization: Bearer ${token}" \\\n  -H "Content-Type: application/json"`
    navigator.clipboard.writeText(cmd)
    alert('cURL command copied to clipboard!')
  }

  async function tryEndpoint(key, method, path) {
    setTesting(prev => ({ ...prev, [key]: true }))
    try {
      const token = localStorage.getItem('kestrel_token')
      const res = await fetch(`/api${path}`, {
        method: method.toUpperCase(),
        headers: {
          'Authorization': token ? `Bearer ${token}` : '',
          'Content-Type': 'application/json',
        },
      })
      const json = await res.json().catch(() => ({ status: res.status, statusText: res.statusText }))
      setTestResult(prev => ({ ...prev, [key]: { status: res.status, body: json } }))
    } catch (err) {
      setTestResult(prev => ({ ...prev, [key]: { error: err.message } }))
    } finally {
      setTesting(prev => ({ ...prev, [key]: false }))
    }
  }

  // Flatten paths into an array of operations
  const operations = []
  if (spec && spec.paths) {
    Object.entries(spec.paths).forEach(([path, methods]) => {
      Object.entries(methods).forEach(([method, op]) => {
        operations.push({
          path,
          method: method.toLowerCase(),
          ...op,
          key: `${method}-${path}`,
        })
      })
    })
  }

  const filteredOps = operations.filter(op => {
    const q = search.toLowerCase()
    return (
      op.path.toLowerCase().includes(q) ||
      op.summary?.toLowerCase().includes(q) ||
      op.tags?.some(t => t.toLowerCase().includes(q))
    )
  })

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0, display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>📜</span> OpenAPI 3.0 Documentation
          </h1>
          <p style={{ color: 'var(--muted)', fontSize: 13, margin: '4px 0 0' }}>
            {spec?.info?.description || 'REST API contracts and interactive request explorer.'}
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <span style={{ fontSize: 12, color: 'var(--muted)', background: 'var(--surface)', padding: '4px 8px', borderRadius: 4, border: '1px solid var(--border)' }}>
            v{spec?.info?.version || '1.0.0'}
          </span>
          <button onClick={fetchSpec} disabled={loading} style={{ fontSize: 12 }}>
            Refresh
          </button>
        </div>
      </div>

      {/* Search Input */}
      <div style={{ marginBottom: 20 }}>
        <input
          type="text"
          placeholder="Filter endpoints by path, summary, or tag (e.g. /skills, auth, project)..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ width: '100%', padding: '10px 14px', fontSize: 13, borderRadius: 6 }}
        />
      </div>

      {loading && <div style={{ color: 'var(--muted)', padding: 24, textAlign: 'center' }}>Loading API specification…</div>}
      {error && <div style={{ color: 'var(--danger)', padding: 16 }}>Failed to load API spec: {error}</div>}

      {/* Endpoints List */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {filteredOps.map(op => {
          const mColor = METHOD_COLORS[op.method] || { bg: 'var(--surface2)', text: 'var(--text)', border: 'var(--border)' }
          const isExp = expanded[op.key]
          const result = testResult[op.key]
          const isTesting = testing[op.key]

          return (
            <div
              key={op.key}
              style={{
                background: 'var(--surface)',
                border: '1px solid var(--border)',
                borderRadius: 8,
                overflow: 'hidden',
              }}
            >
              {/* Endpoint Header Bar */}
              <div
                onClick={() => toggleExpand(op.key)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '12px 16px',
                  cursor: 'pointer',
                  background: isExp ? 'var(--surface2)' : 'transparent',
                  transition: 'background 0.15s',
                }}
              >
                {/* Method Badge */}
                <span style={{
                  textTransform: 'uppercase',
                  fontWeight: 700,
                  fontSize: 12,
                  padding: '3px 8px',
                  borderRadius: 4,
                  background: mColor.bg,
                  color: mColor.text,
                  border: `1px solid ${mColor.border}`,
                  minWidth: 64,
                  textAlign: 'center',
                }}>
                  {op.method}
                </span>

                {/* Path */}
                <span style={{ fontFamily: 'monospace', fontWeight: 600, fontSize: 14 }}>
                  {op.path}
                </span>

                {/* Summary */}
                <span style={{ color: 'var(--muted)', fontSize: 13, marginLeft: 'auto' }}>
                  {op.summary}
                </span>

                {/* Tag pill */}
                {op.tags?.[0] && (
                  <span style={{
                    fontSize: 11,
                    padding: '2px 8px',
                    borderRadius: 12,
                    background: 'rgba(255,255,255,0.06)',
                    color: 'var(--muted)',
                  }}>
                    {op.tags[0]}
                  </span>
                )}
                <span style={{ color: 'var(--muted)', fontSize: 12 }}>{isExp ? '▲' : '▼'}</span>
              </div>

              {/* Expanded details */}
              {isExp && (
                <div style={{ padding: '16px', borderTop: '1px solid var(--border)', display: 'flex', flexDirection: 'column', gap: 14 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div style={{ fontSize: 13, color: 'var(--text)' }}>
                      <strong>Summary:</strong> {op.summary || 'No description provided.'}
                    </div>
                    <div style={{ display: 'flex', gap: 8 }}>
                      <button onClick={() => copyCurl(op.method, op.path)} style={{ fontSize: 12, padding: '4px 10px' }}>
                        📋 Copy cURL
                      </button>
                      {op.method === 'get' && (
                        <button
                          className="primary"
                          onClick={() => tryEndpoint(op.key, op.method, op.path)}
                          disabled={isTesting}
                          style={{ fontSize: 12, padding: '4px 12px' }}
                        >
                          {isTesting ? 'Sending…' : '▶ Execute Test'}
                        </button>
                      )}
                    </div>
                  </div>

                  {/* Responses schema */}
                  {op.responses && (
                    <div>
                      <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--muted)', marginBottom: 6 }}>RESPONSES</div>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                        {Object.entries(op.responses).map(([code, r]) => (
                          <div key={code} style={{ display: 'flex', gap: 8, fontSize: 12 }}>
                            <span style={{ fontFamily: 'monospace', color: 'var(--success)', fontWeight: 600 }}>{code}</span>
                            <span style={{ color: 'var(--muted)' }}>— {r.description}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Execution Response Inspector */}
                  {result && (
                    <div style={{ background: '#090d13', padding: 12, borderRadius: 6, border: '1px solid var(--border)' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6, fontSize: 12 }}>
                        <span style={{ color: 'var(--muted)' }}>Live Response</span>
                        {result.status && (
                          <span style={{ color: result.status < 400 ? 'var(--success)' : 'var(--danger)', fontWeight: 600 }}>
                            HTTP {result.status}
                          </span>
                        )}
                      </div>
                      <pre style={{ margin: 0, fontSize: 12, fontFamily: 'monospace', color: '#c9d1d9', maxHeight: 200, overflowY: 'auto' }}>
                        {JSON.stringify(result.body || result.error, null, 2)}
                      </pre>
                    </div>
                  )}
                </div>
              )}
            </div>
          )
        })}

        {filteredOps.length === 0 && !loading && (
          <div style={{ textAlign: 'center', color: 'var(--muted)', padding: 32 }}>
            No endpoints matched the filter criteria.
          </div>
        )}
      </div>
    </div>
  )
}
