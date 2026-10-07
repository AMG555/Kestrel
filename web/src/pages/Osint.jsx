import React, { useState, useEffect } from 'react'
import { api } from '../api'

const PRESET_QUERIES = {
  fofa: [
    { label: 'Apache Servers', q: 'app="Apache" && country="US"' },
    { label: 'Spring Boot Actuators', q: 'body="actuator/health" && status_code="200"' },
    { label: 'Swagger UI API', q: 'title="Swagger UI"' },
    { label: 'Jenkins Management', q: 'app="Jenkins" && status_code="200"' },
    { label: 'GitLab Login', q: 'app="GitLab" && body="sign_in"' },
  ],
  shodan: [
    { label: 'Open SSH Ports', q: 'port:22 "OpenSSH"' },
    { label: 'Redis Instances', q: 'port:6379 "redis_version"' },
    { label: 'Elasticsearch Clusters', q: 'port:9200 "build_flavor"' },
  ],
}

export default function Osint() {
  const [provider, setProvider] = useState('fofa')
  const [query, setQuery] = useState('title="Swagger UI"')
  const [nlText, setNlText] = useState('')
  const [parsing, setParsing] = useState(false)
  const [searching, setSearching] = useState(false)
  const [results, setResults] = useState([])
  const [total, setTotal] = useState(0)
  const [notice, setNotice] = useState(null)
  const [projects, setProjects] = useState([])
  const [selectedProject, setSelectedProject] = useState('')
  const [importing, setImporting] = useState(false)

  useEffect(() => {
    api.listProjects().then(res => {
      const list = res.projects || res || []
      setProjects(list)
      if (list.length > 0) setSelectedProject(list[0].id)
    }).catch(() => {})
  }, [])

  async function handleParseNL(e) {
    e.preventDefault()
    if (!nlText.trim()) return
    setParsing(true)
    setNotice(null)
    try {
      const res = await api.parseOsintQuery({ provider, text: nlText.trim() })
      if (res.query) {
        setQuery(res.query)
        setNotice({ type: 'info', text: `AI parsed prompt into ${provider.toUpperCase()} syntax: ${res.query}` })
      }
    } catch (err) {
      setNotice({ type: 'error', text: `Failed to convert natural language: ${err.message}` })
    } finally {
      setParsing(false)
    }
  }

  async function handleSearch(e) {
    if (e) e.preventDefault()
    if (!query.trim()) return
    setSearching(true)
    setNotice(null)
    try {
      const res = await api.searchOsint({ provider, query: query.trim(), size: 25 })
      setResults(res.results || [])
      setTotal(res.total || 0)
      if (res.notice) {
        setNotice({ type: 'info', text: res.notice })
      }
    } catch (err) {
      setNotice({ type: 'error', text: `Search failed: ${err.message}` })
    } finally {
      setSearching(false)
    }
  }

  async function handleImport() {
    if (results.length === 0) return
    setImporting(true)
    setNotice(null)
    try {
      const res = await api.importOsintAssets({
        project_id: selectedProject || undefined,
        results: results,
      })
      setNotice({ type: 'success', text: res.message || `Imported ${res.imported_count} assets.` })
    } catch (err) {
      setNotice({ type: 'error', text: `Import failed: ${err.message}` })
    } finally {
      setImporting(false)
    }
  }

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0, display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>🌐</span> Cyberspace Asset Search (OSINT)
          </h1>
          <p style={{ color: 'var(--muted)', fontSize: 13, margin: '4px 0 0' }}>
            Query cyberspace search engines (FOFA, Shodan, ZoomEye, Quake) and directly import targets into project assets.
          </p>
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

      {/* AI Query Builder Card */}
      <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 18, marginBottom: 16 }}>
        <h2 style={{ fontSize: 14, fontWeight: 600, margin: '0 0 10px', color: 'var(--muted)' }}>
          🤖 Natural Language Prompt to Cyberspace Syntax
        </h2>
        <form onSubmit={handleParseNL} style={{ display: 'flex', gap: 8 }}>
          <input
            type="text"
            placeholder="e.g. Find Apache servers in US with login page, or Jenkins running on port 8080..."
            value={nlText}
            onChange={(e) => setNlText(e.target.value)}
            style={{ flex: 1, padding: '8px 12px' }}
          />
          <button type="submit" disabled={parsing || !nlText.trim()} style={{ minWidth: 140 }}>
            {parsing ? 'Translating…' : '✨ Convert to Syntax'}
          </button>
        </form>
      </div>

      {/* Main Query Bar */}
      <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 18, marginBottom: 20 }}>
        <form onSubmit={handleSearch} style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
          <select
            value={provider}
            onChange={(e) => setProvider(e.target.value)}
            style={{ padding: '8px 12px', fontWeight: 600, minWidth: 120 }}
          >
            <option value="fofa">FOFA</option>
            <option value="shodan">Shodan</option>
            <option value="zoomeye">ZoomEye</option>
            <option value="quake">Quake</option>
          </select>
          <input
            type="text"
            placeholder="Enter search syntax (e.g. title=&quot;Swagger UI&quot; or port:443)..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            style={{ flex: 1, padding: '8px 12px', fontFamily: 'monospace' }}
          />
          <button type="submit" className="primary" disabled={searching} style={{ minWidth: 110, padding: '8px 16px' }}>
            {searching ? 'Querying…' : '🔍 Search'}
          </button>
        </form>

        {/* Preset Query Tags */}
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 12, alignItems: 'center' }}>
          <span style={{ fontSize: 12, color: 'var(--muted)' }}>Quick Templates:</span>
          {(PRESET_QUERIES[provider] || PRESET_QUERIES.fofa).map(p => (
            <button
              key={p.label}
              type="button"
              onClick={() => setQuery(p.q)}
              style={{ fontSize: 11, padding: '3px 8px', background: 'var(--surface2)' }}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      {/* Results Table & Import Action */}
      <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, padding: 18 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, margin: 0 }}>
            Discovered Targets ({results.length} displayed / {total} total)
          </h2>
          {results.length > 0 && (
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <select
                value={selectedProject}
                onChange={(e) => setSelectedProject(e.target.value)}
                style={{ fontSize: 12, padding: '5px 8px' }}
              >
                <option value="">Global Inventory (No Project)</option>
                {projects.map(p => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
              <button onClick={handleImport} disabled={importing} className="primary" style={{ fontSize: 12, padding: '5px 14px' }}>
                {importing ? 'Importing…' : '📥 Import All to Assets'}
              </button>
            </div>
          )}
        </div>

        {results.length === 0 ? (
          <div style={{ padding: '32px 0', textAlign: 'center', color: 'var(--muted)', fontSize: 13 }}>
            No results to display. Enter a query above to start reconnaissance.
          </div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--muted)', textAlign: 'left' }}>
                <th style={{ padding: '8px 10px' }}>Host / Target</th>
                <th style={{ padding: '8px 10px' }}>IP</th>
                <th style={{ padding: '8px 10px' }}>Port</th>
                <th style={{ padding: '8px 10px' }}>Protocol</th>
                <th style={{ padding: '8px 10px' }}>Title</th>
                <th style={{ padding: '8px 10px' }}>Server / Banner</th>
                <th style={{ padding: '8px 10px' }}>Country</th>
              </tr>
            </thead>
            <tbody>
              {results.map((r, i) => (
                <tr key={i} style={{ borderBottom: '1px solid var(--border)' }}>
                  <td style={{ padding: '8px 10px', fontFamily: 'monospace', fontWeight: 600 }}>{r.host || '—'}</td>
                  <td style={{ padding: '8px 10px', fontFamily: 'monospace' }}>{r.ip || '—'}</td>
                  <td style={{ padding: '8px 10px' }}>{r.port || '—'}</td>
                  <td style={{ padding: '8px 10px' }}>
                    <span style={{ fontSize: 11, background: 'var(--surface2)', padding: '2px 6px', borderRadius: 4 }}>
                      {r.protocol || 'tcp'}
                    </span>
                  </td>
                  <td style={{ padding: '8px 10px' }}>{r.title || '—'}</td>
                  <td style={{ padding: '8px 10px', color: 'var(--muted)', fontSize: 12 }}>{r.server || '—'}</td>
                  <td style={{ padding: '8px 10px', color: 'var(--muted)', fontSize: 12 }}>{r.country || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
