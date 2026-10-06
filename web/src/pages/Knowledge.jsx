import { useState, useEffect } from 'react'
import { api } from '../api.js'

export default function KnowledgePage() {
  const [docs, setDocs] = useState([])
  const [query, setQuery] = useState('')
  const [results, setResults] = useState([])
  const [text, setText] = useState('')
  const [title, setTitle] = useState('')
  const [ingesting, setIngesting] = useState(false)
  const [querying, setQuerying] = useState(false)
  const [error, setError] = useState('')
  const [enabled, setEnabled] = useState(true)

  const load = () => {
    api.request?.('GET', '/knowledge/documents')
      ?.then?.(d => setDocs(d.documents || []))
      ?.catch?.(err => {
        if (err.message?.includes('not enabled')) setEnabled(false)
        else setError(err.message)
      })
    // Use direct fetch since api.js doesn't expose knowledge yet
    fetch('/api/knowledge/documents', {
      headers: { Authorization: `Bearer ${localStorage.getItem('kestrel_token')}` }
    }).then(r => r.json()).then(d => {
      if (d.error?.includes('not enabled')) setEnabled(false)
      else setDocs(d.documents || [])
    }).catch(() => {})
  }

  useEffect(load, [])

  const ingest = async () => {
    if (!text.trim()) return
    setIngesting(true)
    setError('')
    try {
      const resp = await fetch('/api/knowledge/ingest', {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${localStorage.getItem('kestrel_token')}`,
          'Content-Type': 'text/plain',
          'X-Document-Title': title || 'Untitled',
        },
        body: text,
      })
      const d = await resp.json()
      if (!resp.ok) throw new Error(d.error || 'Ingest failed')
      setText('')
      setTitle('')
      load()
    } catch (err) {
      setError(err.message)
    } finally {
      setIngesting(false)
    }
  }

  const runQuery = async () => {
    if (!query.trim()) return
    setQuerying(true)
    setError('')
    try {
      const resp = await fetch('/api/knowledge/query', {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${localStorage.getItem('kestrel_token')}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ query, top_k: 5 }),
      })
      const d = await resp.json()
      if (!resp.ok) throw new Error(d.error || 'Query failed')
      setResults(d.results || [])
    } catch (err) {
      setError(err.message)
    } finally {
      setQuerying(false)
    }
  }

  const deleteDoc = async (id) => {
    if (!confirm('Delete this document?')) return
    await fetch(`/api/knowledge/documents/${id}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${localStorage.getItem('kestrel_token')}` },
    })
    load()
  }

  if (!enabled) {
    return (
      <div>
        <h1 style={{ fontSize: 20, fontWeight: 700, marginBottom: 16 }}>Knowledge Base</h1>
        <div className="card" style={{ borderColor: 'var(--warn)', background: '#d2992208' }}>
          <div style={{ fontWeight: 600, color: 'var(--warn)', marginBottom: 8 }}>⚠ Knowledge Base Disabled</div>
          <div style={{ color: 'var(--muted)', fontSize: 13 }}>
            Set <code style={{ background: 'var(--surface2)', padding: '1px 6px', borderRadius: 3 }}>knowledge.enabled: true</code> in{' '}
            <code style={{ background: 'var(--surface2)', padding: '1px 6px', borderRadius: 3 }}>config.yaml</code> to enable the RAG pipeline.
          </div>
        </div>
      </div>
    )
  }

  return (
    <div>
      <h1 style={{ fontSize: 20, fontWeight: 700, marginBottom: 16 }}>Knowledge Base</h1>

      {error && <div className="error-msg mb-16">{error}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 24 }}>
        {/* Ingest panel */}
        <div className="card">
          <div style={{ fontWeight: 600, marginBottom: 12 }}>Ingest Document</div>
          <div style={{ marginBottom: 10 }}>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Document Title</label>
            <input value={title} onChange={e => setTitle(e.target.value)} placeholder="My Playbook" />
          </div>
          <div style={{ marginBottom: 10 }}>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Content (plain text / markdown)</label>
            <textarea rows={8} value={text} onChange={e => setText(e.target.value)} placeholder="Paste your playbook, runbook, or documentation here…" />
          </div>
          <button className="primary" onClick={ingest} disabled={ingesting || !text.trim()}>
            {ingesting ? 'Ingesting…' : 'Ingest'}
          </button>
        </div>

        {/* Query panel */}
        <div className="card">
          <div style={{ fontWeight: 600, marginBottom: 12 }}>Retrieve</div>
          <div style={{ marginBottom: 10 }}>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: 12, marginBottom: 4 }}>Query</label>
            <input
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && runQuery()}
              placeholder="How do I enumerate subdomains?"
            />
          </div>
          <button className="primary" onClick={runQuery} disabled={querying || !query.trim()} style={{ marginBottom: 14 }}>
            {querying ? 'Searching…' : 'Search'}
          </button>
          {results.length > 0 && (
            <div style={{ maxHeight: 260, overflowY: 'auto' }}>
              {results.map((r, i) => (
                <div key={i} style={{ padding: '8px 10px', background: 'var(--surface2)', borderRadius: 4, marginBottom: 8, borderLeft: '3px solid var(--accent)' }}>
                  <div style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 4 }}>
                    {r.source} · chunk {r.chunk?.chunk_index} · score {r.score?.toFixed(2)}
                  </div>
                  <div style={{ fontSize: 12, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{r.chunk?.content?.slice(0, 300)}…</div>
                </div>
              ))}
            </div>
          )}
          {results.length === 0 && query && !querying && (
            <div style={{ color: 'var(--muted)', fontSize: 12 }}>No results found.</div>
          )}
        </div>
      </div>

      {/* Document list */}
      <div className="card" style={{ padding: 0 }}>
        <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--border)', fontWeight: 600, fontSize: 13 }}>
          Documents ({docs.length})
        </div>
        <table>
          <thead>
            <tr><th>Title</th><th>Chunks</th><th>Status</th><th>Ingested</th><th></th></tr>
          </thead>
          <tbody>
            {docs.length === 0 && <tr><td colSpan={5} style={{ color: 'var(--muted)', textAlign: 'center', padding: 24 }}>No documents ingested yet.</td></tr>}
            {docs.map(d => (
              <tr key={d.id}>
                <td style={{ fontWeight: 600 }}>{d.title}</td>
                <td style={{ color: 'var(--muted)' }}>{d.chunk_count}</td>
                <td><span className={`badge badge-${d.status === 'indexed' || d.status?.includes('indexed') ? 'active' : 'medium'}`}>{d.status}</span></td>
                <td style={{ color: 'var(--muted)', fontSize: 12 }}>{new Date(d.created_at).toLocaleDateString()}</td>
                <td><button style={{ padding: '3px 8px', fontSize: 11 }} onClick={() => deleteDoc(d.id)}>Delete</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
