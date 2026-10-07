import React, { useState, useEffect } from 'react'
import { api } from '../api'

export default function Skills() {
  const [skills, setSkills] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [search, setSearch] = useState('')
  const [selectedTag, setSelectedTag] = useState('')
  const [selectedSkill, setSelectedSkill] = useState(null)
  const [skillDetail, setSkillDetail] = useState(null)
  const [loadingDetail, setLoadingDetail] = useState(false)

  useEffect(() => {
    loadSkills()
  }, [])

  async function loadSkills() {
    setLoading(true)
    setError(null)
    try {
      const data = await api.listSkills()
      setSkills(data.skills || [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  async function viewSkill(id) {
    setSelectedSkill(id)
    setLoadingDetail(true)
    try {
      const detail = await api.getSkill(id)
      setSkillDetail(detail)
    } catch (err) {
      alert(`Error loading skill: ${err.message}`)
      setSelectedSkill(null)
    } finally {
      setLoadingDetail(false)
    }
  }

  // Collect all unique tags
  const allTags = Array.from(new Set(skills.flatMap(s => s.tags || []))).sort()

  const filtered = skills.filter(s => {
    const matchSearch = !search || 
      s.name.toLowerCase().includes(search.toLowerCase()) ||
      s.dir_name.toLowerCase().includes(search.toLowerCase()) ||
      (s.description && s.description.toLowerCase().includes(search.toLowerCase()))
    const matchTag = !selectedTag || (s.tags && s.tags.includes(selectedTag))
    return matchSearch && matchTag
  })

  return (
    <div style={{ padding: '24px', maxWidth: '1400px', margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '24px' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '26px', fontWeight: 700, display: 'flex', alignItems: 'center', gap: '10px' }}>
            <span>🎯</span> Agent Skills Registry
          </h1>
          <p style={{ margin: '6px 0 0', color: '#94a3b8', fontSize: '14px' }}>
            Modular, progressive-loading skill packages following standard SKILL.md layout for security operations.
          </p>
        </div>
        <button 
          onClick={loadSkills} 
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

      {/* Controls & Filters */}
      <div style={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: '10px', padding: '16px', marginBottom: '24px' }}>
        <div style={{ display: 'flex', gap: '12px', marginBottom: '14px' }}>
          <input
            type="text"
            placeholder="Search skills by name, directory, or keywords..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            style={{
              flex: 1,
              background: '#1e293b',
              border: '1px solid #334155',
              borderRadius: '6px',
              padding: '10px 14px',
              color: '#f8fafc',
              fontSize: '14px'
            }}
          />
          {selectedTag && (
            <button
              onClick={() => setSelectedTag('')}
              style={{
                background: '#334155',
                color: '#cbd5e1',
                border: 'none',
                padding: '0 14px',
                borderRadius: '6px',
                cursor: 'pointer',
                fontSize: '12px'
              }}
            >
              Clear Tag: {selectedTag} ✕
            </button>
          )}
        </div>

        {/* Tag pills */}
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
          {allTags.slice(0, 20).map(tag => (
            <button
              key={tag}
              onClick={() => setSelectedTag(selectedTag === tag ? '' : tag)}
              style={{
                background: selectedTag === tag ? '#2563eb' : '#1e293b',
                color: selectedTag === tag ? '#ffffff' : '#94a3b8',
                border: '1px solid',
                borderColor: selectedTag === tag ? '#3b82f6' : '#334155',
                padding: '4px 10px',
                borderRadius: '16px',
                cursor: 'pointer',
                fontSize: '12px',
                transition: 'all 0.15s ease'
              }}
            >
              #{tag}
            </button>
          ))}
        </div>
      </div>

      {/* Stats Counter */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px', color: '#64748b', fontSize: '13px' }}>
        <span>Showing <strong>{filtered.length}</strong> of <strong>{skills.length}</strong> installed skills</span>
        <span>Standard Agent Skills Layout</span>
      </div>

      {/* Error state */}
      {error && (
        <div style={{ background: '#7f1d1d33', border: '1px solid #dc2626', color: '#fca5a5', padding: '12px', borderRadius: '8px', marginBottom: '20px' }}>
          {error}
        </div>
      )}

      {/* Grid of skills */}
      {loading ? (
        <div style={{ padding: '60px', textAlign: 'center', color: '#64748b' }}>Loading skills...</div>
      ) : filtered.length === 0 ? (
        <div style={{ padding: '60px', textAlign: 'center', color: '#64748b', background: '#0f172a', borderRadius: '10px', border: '1px dashed #334155' }}>
          No skills match your search filters.
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(360px, 1fr))', gap: '16px' }}>
          {filtered.map(skill => (
            <div
              key={skill.id}
              onClick={() => viewSkill(skill.id)}
              style={{
                background: '#0f172a',
                border: '1px solid #1e293b',
                borderRadius: '10px',
                padding: '18px',
                cursor: 'pointer',
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'space-between',
                transition: 'transform 0.15s ease, border-color 0.15s ease',
              }}
              onMouseEnter={e => {
                e.currentTarget.style.borderColor = '#38bdf8'
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
                    {skill.name}
                  </h3>
                  <span style={{ fontSize: '11px', background: '#1e293b', color: '#38bdf8', padding: '2px 8px', borderRadius: '4px', fontFamily: 'monospace' }}>
                    {skill.dir_name}
                  </span>
                </div>
                <p style={{ margin: '0 0 14px', fontSize: '13px', color: '#94a3b8', lineHeight: '1.5', minHeight: '40px' }}>
                  {skill.description || 'No description provided.'}
                </p>
              </div>

              <div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px', marginBottom: '12px' }}>
                  {(skill.tags || []).slice(0, 4).map(t => (
                    <span key={t} style={{ fontSize: '11px', background: '#1e293b', color: '#cbd5e1', padding: '2px 6px', borderRadius: '4px' }}>
                      {t}
                    </span>
                  ))}
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', color: '#64748b', borderTop: '1px solid #1e293b', paddingTop: '10px' }}>
                  <span>{skill.file_count} package files</span>
                  <span style={{ color: '#38bdf8', fontWeight: 500 }}>Inspect Skill →</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Skill Detail Modal */}
      {selectedSkill && (
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
            maxWidth: '900px',
            maxHeight: '90vh',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            boxShadow: '0 25px 50px -12px rgba(0,0,0,0.5)'
          }}>
            {/* Modal Header */}
            <div style={{ padding: '18px 24px', borderBottom: '1px solid #1e293b', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <h2 style={{ margin: 0, fontSize: '18px', color: '#f8fafc', fontWeight: 600 }}>
                  {skillDetail ? skillDetail.name : 'Loading skill...'}
                </h2>
                {skillDetail && (
                  <span style={{ fontSize: '12px', color: '#38bdf8', fontFamily: 'monospace' }}>
                    Path: {skillDetail.path}
                  </span>
                )}
              </div>
              <button
                onClick={() => { setSelectedSkill(null); setSkillDetail(null) }}
                style={{ background: 'transparent', border: 'none', color: '#94a3b8', fontSize: '20px', cursor: 'pointer' }}
              >
                ✕
              </button>
            </div>

            {/* Modal Body */}
            <div style={{ padding: '24px', overflowY: 'auto', flex: 1 }}>
              {loadingDetail || !skillDetail ? (
                <div style={{ textAlign: 'center', padding: '40px', color: '#94a3b8' }}>Loading skill details...</div>
              ) : (
                <div>
                  {/* Metadata banner */}
                  <div style={{ background: '#1e293b', borderRadius: '8px', padding: '14px 18px', marginBottom: '20px' }}>
                    <div style={{ fontSize: '13px', color: '#e2e8f0', marginBottom: '8px' }}>
                      <strong>Description:</strong> {skillDetail.description}
                    </div>
                    {skillDetail.allowed_tools && (
                      <div style={{ fontSize: '12px', color: '#cbd5e1' }}>
                        <strong>Allowed Tools:</strong> <code style={{ color: '#38bdf8' }}>{skillDetail.allowed_tools}</code>
                      </div>
                    )}
                  </div>

                  {/* Sections TOC */}
                  {skillDetail.sections && skillDetail.sections.length > 0 && (
                    <div style={{ marginBottom: '20px' }}>
                      <h4 style={{ margin: '0 0 8px', fontSize: '13px', textTransform: 'uppercase', color: '#64748b' }}>
                        Sections
                      </h4>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
                        {skillDetail.sections.map(sec => (
                          <span key={sec.id} style={{ fontSize: '12px', background: '#0284c722', color: '#38bdf8', padding: '3px 8px', borderRadius: '4px' }}>
                            {sec.title}
                          </span>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Package Files */}
                  {skillDetail.package_files && skillDetail.package_files.length > 0 && (
                    <div style={{ marginBottom: '20px' }}>
                      <h4 style={{ margin: '0 0 8px', fontSize: '13px', textTransform: 'uppercase', color: '#64748b' }}>
                        Package Files ({skillDetail.package_files.length})
                      </h4>
                      <div style={{ background: '#020617', border: '1px solid #1e293b', borderRadius: '6px', padding: '10px', maxHeight: '120px', overflowY: 'auto' }}>
                        {skillDetail.package_files.map(pf => (
                          <div key={pf.path} style={{ fontSize: '12px', color: '#94a3b8', display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
                            <span style={{ fontFamily: 'monospace' }}>{pf.path}</span>
                            <span>{pf.size > 0 ? `${pf.size} B` : ''}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Content Viewer */}
                  <div>
                    <h4 style={{ margin: '0 0 8px', fontSize: '13px', textTransform: 'uppercase', color: '#64748b' }}>
                      Instructions (SKILL.md)
                    </h4>
                    <pre style={{
                      background: '#020617',
                      border: '1px solid #1e293b',
                      borderRadius: '8px',
                      padding: '16px',
                      color: '#e2e8f0',
                      fontSize: '13px',
                      lineHeight: '1.6',
                      overflowX: 'auto',
                      whiteSpace: 'pre-wrap',
                      wordBreak: 'break-word',
                      maxHeight: '400px'
                    }}>
                      {skillDetail.content}
                    </pre>
                  </div>
                </div>
              )}
            </div>

            {/* Modal Footer */}
            <div style={{ padding: '14px 24px', borderTop: '1px solid #1e293b', display: 'flex', justifyContent: 'flex-end', background: '#0b1329' }}>
              <button
                onClick={() => { setSelectedSkill(null); setSkillDetail(null) }}
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
