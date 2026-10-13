import React, { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'

const QUICK_CATEGORIES = [
  { id: '', label: 'All Capabilities' },
  { id: 'recon', label: 'Recon & Surface' },
  { id: 'web', label: 'Web Attacks' },
  { id: 'active-directory', label: 'Active Directory' },
  { id: 'cloud', label: 'Cloud Security' },
  { id: 'binary', label: 'Binary & Mobile' },
  { id: 'post-exploitation', label: 'Post-Exploitation' },
  { id: 'phishing', label: 'Initial Access' },
  { id: 'zero-day', label: 'Zero-Day Discovery' },
  { id: 'ai-llm', label: 'AI & LLM Security' },
  { id: 'blockchain', label: 'Smart Contracts' },
  { id: 'wireless', label: 'Wireless & Hardware' },
  { id: 'source-code', label: 'Source Review' },
  { id: 'pentest', label: 'Standards & OPSEC' },
]

export default function Skills() {
  const [skills, setSkills] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [successMsg, setSuccessMsg] = useState('')
  const [search, setSearch] = useState('')
  const [selectedTag, setSelectedTag] = useState('')
  const [selectedCategory, setSelectedCategory] = useState('')

  // Stats view
  const [showStats, setShowStats] = useState(false)
  const [statsData, setStatsData] = useState(null)
  const [loadingStats, setLoadingStats] = useState(false)

  // View / Edit / Inspect modal state
  const [selectedSkill, setSelectedSkill] = useState(null)
  const [skillDetail, setSkillDetail] = useState(null)
  const [loadingDetail, setLoadingDetail] = useState(false)
  const [modalTab, setModalTab] = useState('doc') // 'doc' | 'files' | 'edit' | 'agent'

  // Package file viewer & editor inside modal
  const [selectedPackageFile, setSelectedPackageFile] = useState(null)
  const [packageFileContent, setPackageFileContent] = useState('')
  const [loadingPackageFile, setLoadingPackageFile] = useState(false)
  const [editingPackageFile, setEditingPackageFile] = useState(false)
  const [editPackageFileContent, setEditPackageFileContent] = useState('')
  const [savingPackageFile, setSavingPackageFile] = useState(false)

  // Edit skill state
  const [editDesc, setEditDesc] = useState('')
  const [editContent, setEditContent] = useState('')
  const [savingEdit, setSavingEdit] = useState(false)

  // Create Skill modal state
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [createForm, setCreateForm] = useState({
    name: '',
    description: '',
    content: `# Security Assessment Playbook\n\n## Objective\nDefine procedural steps and validation criteria.\n\n## Execution Workflow\n1. Initial reconnaissance and discovery\n2. Vulnerability verification\n3. Evidence collection and reporting\n`
  })
  const [creatingSkill, setCreatingSkill] = useState(false)

  useEffect(() => {
    loadSkills()
    loadStats()
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

  async function loadStats() {
    setLoadingStats(true)
    try {
      const data = await api.getSkillStats()
      setStatsData(data)
    } catch (err) {
      // Non-critical, ignore
    } finally {
      setLoadingStats(false)
    }
  }

  async function viewSkill(id) {
    setSelectedSkill(id)
    setLoadingDetail(true)
    setModalTab('doc')
    setSelectedPackageFile(null)
    setPackageFileContent('')
    setEditingPackageFile(false)
    try {
      const detail = await api.getSkill(id)
      setSkillDetail(detail.skill || detail)
      setEditDesc((detail.skill || detail).description || '')
      setEditContent((detail.skill || detail).content || '')
    } catch (err) {
      alert(`Error loading skill: ${err.message}`)
      setSelectedSkill(null)
    } finally {
      setLoadingDetail(false)
    }
  }

  async function loadPackageFile(skillId, filePath) {
    setSelectedPackageFile(filePath)
    setLoadingPackageFile(true)
    setEditingPackageFile(false)
    try {
      const res = await api.getSkillPackageFile(skillId, filePath)
      const content = res.content || ''
      setPackageFileContent(content)
      setEditPackageFileContent(content)
    } catch (err) {
      setPackageFileContent(`[Error loading file: ${err.message}]`)
      setEditPackageFileContent('')
    } finally {
      setLoadingPackageFile(false)
    }
  }

  async function handleSavePackageFile() {
    if (!selectedSkill || !selectedPackageFile) return
    setSavingPackageFile(true)
    try {
      await api.putSkillPackageFile(selectedSkill, selectedPackageFile, editPackageFileContent)
      setPackageFileContent(editPackageFileContent)
      setEditingPackageFile(false)
      notifySuccess(`Saved file '${selectedPackageFile}'`)
    } catch (err) {
      alert(`Failed to save package file: ${err.message}`)
    } finally {
      setSavingPackageFile(false)
    }
  }

  async function handleSaveSkill() {
    if (!selectedSkill) return
    setSavingEdit(true)
    setError(null)
    try {
      await api.updateSkill(selectedSkill, {
        description: editDesc,
        content: editContent
      })
      notifySuccess(`Skill '${selectedSkill}' updated successfully`)
      await viewSkill(selectedSkill)
      await loadSkills()
    } catch (err) {
      alert(`Failed to save skill: ${err.message}`)
    } finally {
      setSavingEdit(false)
    }
  }

  async function handleDeleteSkill(skillId) {
    if (!window.confirm(`Are you sure you want to delete skill package '${skillId}'? This action cannot be undone.`)) {
      return
    }
    try {
      await api.deleteSkill(skillId)
      notifySuccess(`Skill '${skillId}' deleted`)
      setSelectedSkill(null)
      setSkillDetail(null)
      await loadSkills()
      await loadStats()
    } catch (err) {
      alert(`Failed to delete skill: ${err.message}`)
    }
  }

  async function handleCreateSkill(e) {
    e.preventDefault()
    const name = createForm.name.trim().toLowerCase()
    if (!name) return

    setCreatingSkill(true)
    try {
      await api.createSkill({
        name,
        description: createForm.description.trim(),
        content: createForm.content
      })
      notifySuccess(`New skill '${name}' created successfully`)
      setShowCreateModal(false)
      setCreateForm({
        name: '',
        description: '',
        content: `# Security Assessment Playbook\n\n## Objective\nDefine procedural steps and validation criteria.\n\n## Execution Workflow\n1. Initial reconnaissance and discovery\n2. Vulnerability verification\n3. Evidence collection and reporting\n`
      })
      await loadSkills()
      await loadStats()
    } catch (err) {
      alert(`Failed to create skill: ${err.message}`)
    } finally {
      setCreatingSkill(false)
    }
  }

  async function handleClearAllStats() {
    if (!window.confirm('Clear all skill call statistics records?')) return
    try {
      await api.clearSkillStats()
      notifySuccess('Skill statistics cleared')
      await loadStats()
    } catch (err) {
      alert(`Error clearing statistics: ${err.message}`)
    }
  }

  async function handleClearSingleSkillStats(skillName) {
    try {
      await api.clearSkillStatsByName(skillName)
      notifySuccess(`Statistics for '${skillName}' cleared`)
      await loadStats()
    } catch (err) {
      alert(`Error clearing stats: ${err.message}`)
    }
  }

  function notifySuccess(msg) {
    setSuccessMsg(msg)
    setTimeout(() => setSuccessMsg(''), 4000)
  }

  function copyPromptToClipboard(skillName, skillDesc) {
    const promptText = `Please activate and execute the "${skillName}" skill (${skillDesc || 'security assessment'}). Follow its progressive instructions and standard verification.`
    navigator.clipboard.writeText(promptText)
    notifySuccess(`Prompt copied for '${skillName}'! Paste in the Agent tab.`)
  }

  // Collect all unique tags
  const allTags = Array.from(new Set(skills.flatMap(s => s.tags || []))).sort()

  const filtered = skills.filter(s => {
    const matchSearch = !search || 
      s.name.toLowerCase().includes(search.toLowerCase()) ||
      s.dir_name.toLowerCase().includes(search.toLowerCase()) ||
      (s.description && s.description.toLowerCase().includes(search.toLowerCase())) ||
      (s.triggers && s.triggers.some(t => t.toLowerCase().includes(search.toLowerCase())))

    const matchTag = !selectedTag || (s.tags && s.tags.includes(selectedTag))

    const matchCategory = !selectedCategory || 
      s.name.toLowerCase().includes(selectedCategory) ||
      s.dir_name.toLowerCase().includes(selectedCategory) ||
      (s.description && s.description.toLowerCase().includes(selectedCategory)) ||
      (s.tags && s.tags.some(t => t.toLowerCase().includes(selectedCategory)))

    return matchSearch && matchTag && matchCategory
  })

  return (
    <div style={{ maxWidth: '1440px', margin: '0 auto' }}>
      {/* Page Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '24px', flexWrap: 'wrap', gap: '16px' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '24px', fontWeight: 700, display: 'flex', alignItems: 'center', gap: '10px' }}>
            <span>🎯</span> Agent Skills Registry
            <span style={{ fontSize: '11px', background: 'rgba(56, 189, 248, 0.15)', color: '#38bdf8', padding: '3px 8px', borderRadius: '12px', fontWeight: 600 }}>
              Eino ADK Spec
            </span>
          </h1>
          <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: '13px', lineHeight: 1.5 }}>
            Modular, progressive-loading skill packages loaded dynamically by the multi-agent AI loop for specialized security workflows.
          </p>
        </div>

        <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
          <button 
            onClick={() => setShowStats(!showStats)} 
            style={{
              background: showStats ? 'var(--accent)' : 'var(--surface2)',
              color: showStats ? '#0d1117' : 'var(--text)',
              border: '1px solid var(--border)',
              padding: '7px 14px',
              borderRadius: '6px',
              fontSize: '13px',
              fontWeight: 500
            }}
          >
            📊 {showStats ? 'Hide Stats' : 'Call Telemetry'}
          </button>

          <button 
            onClick={() => setShowCreateModal(true)} 
            className="primary"
            style={{
              padding: '7px 16px',
              borderRadius: '6px',
              fontSize: '13px',
              fontWeight: 600
            }}
          >
            + Create Skill
          </button>

          <button 
            onClick={() => { loadSkills(); loadStats(); }} 
            style={{
              background: 'var(--surface2)',
              color: 'var(--text)',
              border: '1px solid var(--border)',
              padding: '7px 14px',
              borderRadius: '6px',
              fontSize: '13px',
              fontWeight: 500
            }}
          >
            ↻ Refresh
          </button>
        </div>
      </div>

      {/* Success Notification */}
      {successMsg && (
        <div style={{ background: 'rgba(34, 197, 94, 0.15)', border: '1px solid #22c55e', color: '#4ade80', padding: '12px 16px', borderRadius: '8px', marginBottom: '20px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>✓ {successMsg}</span>
          <button onClick={() => setSuccessMsg('')} style={{ background: 'none', border: 'none', color: '#4ade80', cursor: 'pointer' }}>✕</button>
        </div>
      )}

      {/* Error state */}
      {error && (
        <div style={{ background: '#7f1d1d33', border: '1px solid #dc2626', color: '#fca5a5', padding: '12px 16px', borderRadius: '8px', marginBottom: '20px' }}>
          ⚠ {error}
        </div>
      )}

      {/* Metrics Banner */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '12px', marginBottom: '20px' }}>
        <div className="card" style={{ padding: '14px 16px' }}>
          <div style={{ fontSize: '11px', color: 'var(--muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Installed Skills</div>
          <div style={{ fontSize: '24px', fontWeight: 700, color: 'var(--accent)', marginTop: '4px' }}>
            {skills.length}
          </div>
          <div style={{ fontSize: '11px', color: '#22c55e', marginTop: '2px' }}>✓ All packages validated</div>
        </div>

        <div className="card" style={{ padding: '14px 16px' }}>
          <div style={{ fontSize: '11px', color: 'var(--muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>ADK Runtime Middleware</div>
          <div style={{ fontSize: '16px', fontWeight: 600, color: '#38bdf8', marginTop: '6px' }}>
            ● Progressive Loading
          </div>
          <div style={{ fontSize: '11px', color: 'var(--muted)', marginTop: '2px' }}>Disclosed via 'skill' tool</div>
        </div>

        <div className="card" style={{ padding: '14px 16px' }}>
          <div style={{ fontSize: '11px', color: 'var(--muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Total Invocations</div>
          <div style={{ fontSize: '24px', fontWeight: 700, color: '#f59e0b', marginTop: '4px' }}>
            {statsData?.total_calls ?? 0}
          </div>
          <div style={{ fontSize: '11px', color: 'var(--muted)', marginTop: '2px' }}>
            {statsData?.total_success ?? 0} succeeded · {statsData?.total_failed ?? 0} failed
          </div>
        </div>

        <div className="card" style={{ padding: '14px 16px' }}>
          <div style={{ fontSize: '11px', color: 'var(--muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Storage Directory</div>
          <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginTop: '8px', fontFamily: 'monospace' }}>
            {statsData?.skills_dir || 'skills/'}
          </div>
          <div style={{ fontSize: '11px', color: 'var(--muted)', marginTop: '2px' }}>Agent Skills layout</div>
        </div>
      </div>

      {/* Stats Table Panel (collapsible) */}
      {showStats && (
        <div className="card" style={{ marginBottom: '24px', background: 'var(--surface)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '14px' }}>
            <h3 style={{ margin: 0, fontSize: '15px', fontWeight: 600 }}>
              📊 Skill Invocation Statistics
            </h3>
            <button 
              onClick={handleClearAllStats} 
              style={{ fontSize: '11px', color: 'var(--danger)', background: 'transparent', border: '1px solid var(--border)' }}
            >
              Clear All Stats
            </button>
          </div>

          {loadingStats ? (
            <div style={{ padding: '20px', textAlign: 'center', color: 'var(--muted)' }}>Loading stats...</div>
          ) : !statsData?.stats || statsData.stats.length === 0 ? (
            <div style={{ padding: '20px', textAlign: 'center', color: 'var(--muted)' }}>No call records logged yet.</div>
          ) : (
            <div style={{ maxHeight: '280px', overflowY: 'auto' }}>
              <table>
                <thead>
                  <tr>
                    <th>Skill</th>
                    <th>Total Calls</th>
                    <th>Success</th>
                    <th>Failed</th>
                    <th>Last Called</th>
                    <th>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {statsData.stats.map(st => (
                    <tr key={st.skill_name}>
                      <td style={{ fontWeight: 600, fontFamily: 'monospace' }}>{st.skill_name}</td>
                      <td>{st.total_calls}</td>
                      <td style={{ color: '#22c55e' }}>{st.success_calls}</td>
                      <td style={{ color: st.failed_calls > 0 ? 'var(--danger)' : 'var(--muted)' }}>{st.failed_calls}</td>
                      <td style={{ color: 'var(--muted)', fontSize: '12px' }}>{st.last_call_time || 'Never'}</td>
                      <td>
                        {st.total_calls > 0 && (
                          <button 
                            onClick={() => handleClearSingleSkillStats(st.skill_name)}
                            style={{ fontSize: '10px', padding: '2px 6px' }}
                          >
                            Reset
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Search & Category Filter Bar */}
      <div className="card" style={{ padding: '16px', marginBottom: '20px' }}>
        <div style={{ display: 'flex', gap: '12px', marginBottom: '14px' }}>
          <input
            type="text"
            placeholder="Search skills by name, directory, triggers, or keywords..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            style={{
              flex: 1,
              background: 'var(--surface2)',
              border: '1px solid var(--border)',
              borderRadius: '6px',
              padding: '10px 14px',
              color: 'var(--text)',
              fontSize: '14px'
            }}
          />
          {(search || selectedTag || selectedCategory) && (
            <button
              onClick={() => { setSearch(''); setSelectedTag(''); setSelectedCategory('') }}
              style={{
                background: 'var(--surface2)',
                color: 'var(--muted)',
                padding: '0 16px',
                fontSize: '12px',
                borderRadius: '6px'
              }}
            >
              Reset Filters ✕
            </button>
          )}
        </div>

        {/* Capability / Category Pills */}
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px', marginBottom: '10px' }}>
          {QUICK_CATEGORIES.map(cat => (
            <button
              key={cat.id}
              onClick={() => setSelectedCategory(selectedCategory === cat.id ? '' : cat.id)}
              style={{
                background: selectedCategory === cat.id ? 'var(--accent)' : 'var(--surface2)',
                color: selectedCategory === cat.id ? '#0d1117' : 'var(--text)',
                borderColor: selectedCategory === cat.id ? 'var(--accent)' : 'var(--border)',
                fontWeight: selectedCategory === cat.id ? 600 : 400,
                padding: '4px 12px',
                borderRadius: '16px',
                fontSize: '12px',
                transition: 'all 0.15s ease'
              }}
            >
              {cat.label}
            </button>
          ))}
        </div>

        {/* Tag pills */}
        {allTags.length > 0 && (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px', borderTop: '1px solid var(--border)', paddingTop: '10px' }}>
            <span style={{ fontSize: '11px', color: 'var(--muted)', alignSelf: 'center', marginRight: '4px' }}>Tags:</span>
            {allTags.slice(0, 24).map(tag => (
              <button
                key={tag}
                onClick={() => setSelectedTag(selectedTag === tag ? '' : tag)}
                style={{
                  background: selectedTag === tag ? '#0284c7' : 'transparent',
                  color: selectedTag === tag ? '#fff' : 'var(--muted)',
                  border: '1px solid',
                  borderColor: selectedTag === tag ? '#0284c7' : 'var(--border)',
                  padding: '2px 8px',
                  borderRadius: '12px',
                  fontSize: '11px',
                  cursor: 'pointer'
                }}
              >
                #{tag}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Count & Layout Information */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '14px', color: 'var(--muted)', fontSize: '13px' }}>
        <span>Showing <strong>{filtered.length}</strong> of <strong>{skills.length}</strong> available skills</span>
        <span>Standard Agent Skills Layout (SKILL.md)</span>
      </div>

      {/* Grid of skills */}
      {loading ? (
        <div style={{ padding: '60px', textAlign: 'center', color: 'var(--muted)' }}>Loading skills catalog...</div>
      ) : filtered.length === 0 ? (
        <div style={{ padding: '60px', textAlign: 'center', color: 'var(--muted)', background: 'var(--surface)', borderRadius: '10px', border: '1px dashed var(--border)' }}>
          No skills match your search filters.
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(360px, 1fr))', gap: '16px' }}>
          {filtered.map(skill => (
            <div
              key={skill.id}
              style={{
                background: 'var(--surface)',
                border: '1px solid var(--border)',
                borderRadius: '10px',
                padding: '18px',
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'space-between',
                transition: 'border-color 0.15s ease, transform 0.15s ease',
              }}
              onMouseEnter={e => {
                e.currentTarget.style.borderColor = 'var(--accent)'
                e.currentTarget.style.transform = 'translateY(-2px)'
              }}
              onMouseLeave={e => {
                e.currentTarget.style.borderColor = 'var(--border)'
                e.currentTarget.style.transform = 'none'
              }}
            >
              <div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '8px' }}>
                  <h3 
                    onClick={() => viewSkill(skill.id)}
                    style={{ margin: 0, fontSize: '16px', fontWeight: 600, color: 'var(--text)', cursor: 'pointer' }}
                  >
                    {skill.name}
                  </h3>
                  <span style={{ fontSize: '11px', background: 'var(--surface2)', color: 'var(--accent)', padding: '2px 8px', borderRadius: '4px', fontFamily: 'monospace' }}>
                    {skill.dir_name}
                  </span>
                </div>
                <p 
                  onClick={() => viewSkill(skill.id)}
                  style={{ margin: '0 0 14px', fontSize: '13px', color: 'var(--muted)', lineHeight: '1.5', minHeight: '40px', cursor: 'pointer' }}
                >
                  {skill.description || 'No description provided.'}
                </p>
              </div>

              <div>
                {/* Tags */}
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px', marginBottom: '12px' }}>
                  {(skill.tags || []).slice(0, 4).map(t => (
                    <span key={t} style={{ fontSize: '11px', background: 'var(--surface2)', color: 'var(--text)', padding: '2px 6px', borderRadius: '4px' }}>
                      {t}
                    </span>
                  ))}
                  {skill.file_count > 1 && (
                    <span style={{ fontSize: '11px', background: 'rgba(56, 189, 248, 0.1)', color: '#38bdf8', padding: '2px 6px', borderRadius: '4px' }}>
                      {skill.file_count} files
                    </span>
                  )}
                </div>

                {/* Action footer */}
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '12px', borderTop: '1px solid var(--border)', paddingTop: '10px' }}>
                  <button
                    onClick={() => copyPromptToClipboard(skill.name, skill.description)}
                    style={{ background: 'transparent', border: 'none', color: 'var(--muted)', padding: 0, fontSize: '12px', cursor: 'pointer' }}
                    title="Copy Agent prompt command to clipboard"
                  >
                    📋 Copy Prompt
                  </button>

                  <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
                    <Link
                      to={`/agent?mode=deep&intent=${encodeURIComponent(`Please activate and execute the "${skill.name}" skill. Perform thorough assessment following its guidelines.`)}`}
                      style={{ color: '#38bdf8', textDecoration: 'none', fontWeight: 600, fontSize: '12px', display: 'inline-flex', alignItems: 'center', gap: '3px' }}
                      title="Launch immediately in DeepAgent orchestrator"
                    >
                      ▶ Run in Agent
                    </Link>
                    <button
                      onClick={() => viewSkill(skill.id)}
                      style={{ background: 'transparent', border: 'none', color: 'var(--accent)', fontWeight: 500, padding: 0, cursor: 'pointer' }}
                    >
                      Inspect →
                    </button>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Skill Detail & Inspection Modal */}
      {selectedSkill && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0,0,0,0.8)',
          backdropFilter: 'blur(4px)',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          zIndex: 1000,
          padding: '20px'
        }}>
          <div style={{
            background: 'var(--surface)',
            border: '1px solid var(--border)',
            borderRadius: '12px',
            width: '100%',
            maxWidth: '960px',
            maxHeight: '92vh',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            boxShadow: '0 25px 50px -12px rgba(0,0,0,0.7)'
          }}>
            {/* Modal Header */}
            <div style={{ padding: '18px 24px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <h2 style={{ margin: 0, fontSize: '18px', color: 'var(--text)', fontWeight: 600, display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span>🎯</span> {skillDetail ? skillDetail.name : 'Loading skill...'}
                </h2>
                {skillDetail && (
                  <div style={{ fontSize: '12px', color: 'var(--muted)', marginTop: '4px', fontFamily: 'monospace' }}>
                    Path: {skillDetail.path}
                  </div>
                )}
              </div>
              <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
                <button
                  onClick={() => handleDeleteSkill(selectedSkill)}
                  style={{ background: 'transparent', border: '1px solid rgba(239,68,68,0.4)', color: 'var(--danger)', fontSize: '12px', padding: '4px 10px', borderRadius: '4px' }}
                >
                  Delete Skill
                </button>
                <button
                  onClick={() => { setSelectedSkill(null); setSkillDetail(null); setSelectedPackageFile(null) }}
                  style={{ background: 'transparent', border: 'none', color: 'var(--muted)', fontSize: '20px', cursor: 'pointer' }}
                >
                  ✕
                </button>
              </div>
            </div>

            {/* Modal Navigation Tabs */}
            <div style={{ display: 'flex', borderBottom: '1px solid var(--border)', background: 'var(--surface2)', padding: '0 24px' }}>
              {[
                { id: 'doc', label: '📖 SKILL.md Instructions' },
                { id: 'files', label: `📦 Package Files (${skillDetail?.package_files?.length || 0})` },
                { id: 'edit', label: '✏️ Edit Package' },
                { id: 'agent', label: '🤖 Agent Activation Guide' },
              ].map(tab => (
                <button
                  key={tab.id}
                  onClick={() => setModalTab(tab.id)}
                  style={{
                    background: 'transparent',
                    border: 'none',
                    borderBottom: modalTab === tab.id ? '2px solid var(--accent)' : '2px solid transparent',
                    color: modalTab === tab.id ? 'var(--accent)' : 'var(--muted)',
                    padding: '12px 16px',
                    fontSize: '13px',
                    fontWeight: modalTab === tab.id ? 600 : 400,
                    cursor: 'pointer',
                    borderRadius: 0
                  }}
                >
                  {tab.label}
                </button>
              ))}
            </div>

            {/* Modal Body */}
            <div style={{ padding: '24px', overflowY: 'auto', flex: 1 }}>
              {loadingDetail || !skillDetail ? (
                <div style={{ textAlign: 'center', padding: '40px', color: 'var(--muted)' }}>Loading skill details...</div>
              ) : (
                <>
                  {/* TAB 1: Documentation / Instructions */}
                  {modalTab === 'doc' && (
                    <div>
                      {/* Metadata Banner */}
                      <div style={{ background: 'var(--surface2)', borderRadius: '8px', padding: '14px 18px', marginBottom: '20px' }}>
                        <div style={{ fontSize: '13px', color: 'var(--text)', marginBottom: '8px' }}>
                          <strong>Description:</strong> {skillDetail.description}
                        </div>
                        {skillDetail.allowed_tools && (
                          <div style={{ fontSize: '12px', color: 'var(--muted)' }}>
                            <strong>Allowed Tools:</strong> <code style={{ color: 'var(--accent)' }}>{skillDetail.allowed_tools}</code>
                          </div>
                        )}
                      </div>

                      {/* Sections */}
                      {skillDetail.sections && skillDetail.sections.length > 0 && (
                        <div style={{ marginBottom: '20px' }}>
                          <h4 style={{ margin: '0 0 8px', fontSize: '12px', textTransform: 'uppercase', color: 'var(--muted)' }}>
                            Declared Sections
                          </h4>
                          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
                            {skillDetail.sections.map(sec => (
                              <span key={sec.id} style={{ fontSize: '12px', background: 'rgba(56,189,248,0.1)', color: '#38bdf8', padding: '3px 8px', borderRadius: '4px' }}>
                                {sec.title}
                              </span>
                            ))}
                          </div>
                        </div>
                      )}

                      {/* Content Viewer */}
                      <div>
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
                          <h4 style={{ margin: 0, fontSize: '12px', textTransform: 'uppercase', color: 'var(--muted)' }}>
                            SKILL.md Content
                          </h4>
                          <button
                            onClick={() => {
                              navigator.clipboard.writeText(skillDetail.content || '')
                              notifySuccess('SKILL.md copied to clipboard')
                            }}
                            style={{ fontSize: '11px', padding: '3px 8px' }}
                          >
                            Copy Content
                          </button>
                        </div>
                        <pre style={{
                          background: '#090d13',
                          border: '1px solid var(--border)',
                          borderRadius: '8px',
                          padding: '16px',
                          color: 'var(--text)',
                          fontSize: '13px',
                          lineHeight: '1.6',
                          overflowX: 'auto',
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-word',
                          maxHeight: '440px'
                        }}>
                          {skillDetail.content}
                        </pre>
                      </div>
                    </div>
                  )}

                  {/* TAB 2: Package Files Explorer */}
                  {modalTab === 'files' && (
                    <div>
                      <p style={{ margin: '0 0 16px', fontSize: '13px', color: 'var(--muted)' }}>
                        Agent Skill packages can contain executable scripts, payloads, templates, and reference manuals. Click any file to view content.
                      </p>

                      <div style={{ display: 'grid', gridTemplateColumns: '260px 1fr', gap: '16px', minHeight: '360px' }}>
                        {/* File Tree / List */}
                        <div style={{ background: '#090d13', border: '1px solid var(--border)', borderRadius: '8px', padding: '8px', maxHeight: '420px', overflowY: 'auto' }}>
                          <div style={{ padding: '4px 8px', fontSize: '11px', textTransform: 'uppercase', color: 'var(--muted)', fontWeight: 600 }}>
                            Files
                          </div>
                          {skillDetail.package_files && skillDetail.package_files.length > 0 ? (
                            skillDetail.package_files.map(pf => (
                              <div
                                key={pf.path}
                                onClick={() => loadPackageFile(selectedSkill, pf.path)}
                                style={{
                                  padding: '8px 10px',
                                  borderRadius: '4px',
                                  cursor: 'pointer',
                                  fontSize: '12px',
                                  fontFamily: 'monospace',
                                  background: selectedPackageFile === pf.path ? 'var(--surface2)' : 'transparent',
                                  color: selectedPackageFile === pf.path ? 'var(--accent)' : 'var(--text)',
                                  display: 'flex',
                                  justifyContent: 'space-between',
                                  alignItems: 'center',
                                  marginBottom: '2px'
                                }}
                              >
                                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                  📄 {pf.path}
                                </span>
                                <span style={{ fontSize: '10px', color: 'var(--muted)' }}>
                                  {pf.size > 0 ? `${pf.size} B` : ''}
                                </span>
                              </div>
                            ))
                          ) : (
                            <div style={{ padding: '16px', color: 'var(--muted)', fontSize: '12px' }}>
                              Only SKILL.md in this package.
                            </div>
                          )}
                        </div>

                        {/* File Content Preview / Editor */}
                        <div style={{ background: '#090d13', border: '1px solid var(--border)', borderRadius: '8px', padding: '14px', display: 'flex', flexDirection: 'column' }}>
                          {selectedPackageFile ? (
                            <>
                              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px', borderBottom: '1px solid var(--border)', paddingBottom: '8px' }}>
                                <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--accent)', fontFamily: 'monospace' }}>
                                  {selectedPackageFile}
                                </span>
                                <div style={{ display: 'flex', gap: '6px' }}>
                                  {!editingPackageFile ? (
                                    <>
                                      <button
                                        onClick={() => {
                                          navigator.clipboard.writeText(packageFileContent)
                                          notifySuccess(`Copied ${selectedPackageFile}`)
                                        }}
                                        style={{ fontSize: '11px', padding: '2px 8px' }}
                                      >
                                        Copy
                                      </button>
                                      <button
                                        onClick={() => {
                                          setEditPackageFileContent(packageFileContent)
                                          setEditingPackageFile(true)
                                        }}
                                        style={{ fontSize: '11px', padding: '2px 8px', color: 'var(--accent)' }}
                                      >
                                        ✏️ Edit
                                      </button>
                                    </>
                                  ) : (
                                    <>
                                      <button
                                        onClick={() => setEditingPackageFile(false)}
                                        disabled={savingPackageFile}
                                        style={{ fontSize: '11px', padding: '2px 8px' }}
                                      >
                                        Cancel
                                      </button>
                                      <button
                                        onClick={handleSavePackageFile}
                                        disabled={savingPackageFile}
                                        className="primary"
                                        style={{ fontSize: '11px', padding: '2px 8px' }}
                                      >
                                        {savingPackageFile ? 'Saving…' : 'Save'}
                                      </button>
                                    </>
                                  )}
                                </div>
                              </div>
                              {loadingPackageFile ? (
                                <div style={{ padding: '40px', textAlign: 'center', color: 'var(--muted)' }}>Loading file content...</div>
                              ) : editingPackageFile ? (
                                <textarea
                                  rows={14}
                                  value={editPackageFileContent}
                                  onChange={e => setEditPackageFileContent(e.target.value)}
                                  style={{
                                    width: '100%',
                                    boxSizing: 'border-box',
                                    background: '#04070b',
                                    border: '1px solid var(--border)',
                                    borderRadius: '4px',
                                    padding: '10px',
                                    color: 'var(--text)',
                                    fontSize: '12px',
                                    fontFamily: 'monospace',
                                    lineHeight: '1.5',
                                    flex: 1,
                                    resize: 'vertical'
                                  }}
                                />
                              ) : (
                                <pre style={{
                                  margin: 0,
                                  fontSize: '12px',
                                  lineHeight: '1.5',
                                  color: 'var(--text)',
                                  overflowY: 'auto',
                                  flex: 1,
                                  whiteSpace: 'pre-wrap',
                                  wordBreak: 'break-word',
                                  maxHeight: '360px'
                                }}>
                                  {packageFileContent}
                                </pre>
                              )}
                            </>
                          ) : (
                            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--muted)', fontSize: '13px' }}>
                              Select a file from the list to view or edit its contents.
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  )}

                  {/* TAB 3: Edit Skill */}
                  {modalTab === 'edit' && (
                    <div>
                      <div style={{ marginBottom: '14px' }}>
                        <label style={{ display: 'block', fontSize: '12px', color: 'var(--muted)', marginBottom: '4px' }}>
                          Description
                        </label>
                        <input
                          type="text"
                          value={editDesc}
                          onChange={e => setEditDesc(e.target.value)}
                          style={{
                            width: '100%',
                            boxSizing: 'border-box',
                            background: 'var(--surface2)',
                            border: '1px solid var(--border)',
                            borderRadius: '6px',
                            padding: '8px 12px',
                            color: 'var(--text)',
                            fontSize: '13px'
                          }}
                        />
                      </div>

                      <div style={{ marginBottom: '16px' }}>
                        <label style={{ display: 'block', fontSize: '12px', color: 'var(--muted)', marginBottom: '4px' }}>
                          SKILL.md Content (YAML Front Matter + Markdown body)
                        </label>
                        <textarea
                          rows={14}
                          value={editContent}
                          onChange={e => setEditContent(e.target.value)}
                          style={{
                            width: '100%',
                            boxSizing: 'border-box',
                            background: '#090d13',
                            border: '1px solid var(--border)',
                            borderRadius: '6px',
                            padding: '12px',
                            color: 'var(--text)',
                            fontSize: '13px',
                            fontFamily: 'monospace',
                            lineHeight: '1.5'
                          }}
                        />
                      </div>

                      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                        <button
                          onClick={handleSaveSkill}
                          disabled={savingEdit}
                          className="primary"
                          style={{ padding: '8px 20px', fontSize: '13px' }}
                        >
                          {savingEdit ? 'Saving…' : 'Save Changes'}
                        </button>
                      </div>
                    </div>
                  )}

                  {/* TAB 4: Agent Activation Guide */}
                  {modalTab === 'agent' && (
                    <div>
                      <div className="card" style={{ background: 'var(--surface2)', marginBottom: '16px' }}>
                        <h4 style={{ margin: '0 0 8px', fontSize: '14px', color: 'var(--accent)' }}>
                          🤖 How Kestrel Activates This Skill
                        </h4>
                        <p style={{ margin: '0 0 10px', fontSize: '13px', lineHeight: 1.6 }}>
                          Kestrel uses CloudWeGo Eino ADK skill middleware. The model sees the name and description of all registered skills in its system context.
                          When an assessment task touches this domain, the model calls the built-in <code style={{ color: 'var(--accent)' }}>skill</code> tool with parameter:
                        </p>
                        <pre style={{ background: '#090d13', padding: '10px 14px', borderRadius: '6px', fontSize: '12px', color: '#38bdf8' }}>
{`skill(name: "${skillDetail.id || skillDetail.name}")`}
                        </pre>
                        <p style={{ margin: '10px 0 0', fontSize: '13px', lineHeight: 1.6 }}>
                          The middleware returns the full <code style={{ color: 'var(--accent)' }}>SKILL.md</code> instructions, and filesystem tools are made available to execute any scripts in the skill package.
                        </p>
                      </div>

                      <div className="card">
                        <h4 style={{ margin: '0 0 8px', fontSize: '14px' }}>
                          📋 Trigger in Agent Chat
                        </h4>
                        <p style={{ margin: '0 0 10px', fontSize: '13px', color: 'var(--muted)' }}>
                          Copy the recommended trigger prompt to immediately direct Kestrel to invoke this skill:
                        </p>
                        <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' }}>
                          <input
                            readOnly
                            value={`Please activate and execute the "${skillDetail.name}" skill. Perform thorough assessment following its guidelines.`}
                            style={{ flex: 1, minWidth: '280px', background: '#090d13', fontFamily: 'monospace', fontSize: '12px' }}
                          />
                          <button
                            onClick={() => copyPromptToClipboard(skillDetail.name, skillDetail.description)}
                            style={{ padding: '7px 12px', fontSize: '12px' }}
                          >
                            Copy Prompt
                          </button>
                          <Link
                            to={`/agent?mode=deep&intent=${encodeURIComponent(`Please activate and execute the "${skillDetail.name}" skill. Perform thorough assessment following its guidelines.`)}`}
                            className="primary"
                            style={{ padding: '7px 14px', fontSize: '12px', textDecoration: 'none', display: 'inline-flex', alignItems: 'center', gap: '4px' }}
                          >
                            ▶ Launch in DeepAgent
                          </Link>
                        </div>
                      </div>
                    </div>
                  )}
                </>
              )}
            </div>

            {/* Modal Footer */}
            <div style={{ padding: '14px 24px', borderTop: '1px solid var(--border)', display: 'flex', justifyContent: 'flex-end', background: 'var(--surface2)' }}>
              <button
                onClick={() => { setSelectedSkill(null); setSkillDetail(null); setSelectedPackageFile(null) }}
                style={{
                  background: 'var(--border)',
                  color: 'var(--text)',
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

      {/* Create Skill Modal */}
      {showCreateModal && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0,0,0,0.8)',
          backdropFilter: 'blur(4px)',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          zIndex: 1000,
          padding: '20px'
        }}>
          <div style={{
            background: 'var(--surface)',
            border: '1px solid var(--border)',
            borderRadius: '12px',
            width: '100%',
            maxWidth: '680px',
            maxHeight: '90vh',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            boxShadow: '0 25px 50px -12px rgba(0,0,0,0.7)'
          }}>
            <div style={{ padding: '18px 24px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h2 style={{ margin: 0, fontSize: '18px', color: 'var(--text)', fontWeight: 600 }}>
                + Create Custom Agent Skill
              </h2>
              <button
                onClick={() => setShowCreateModal(false)}
                style={{ background: 'transparent', border: 'none', color: 'var(--muted)', fontSize: '20px', cursor: 'pointer' }}
              >
                ✕
              </button>
            </div>

            <form onSubmit={handleCreateSkill} style={{ padding: '24px', overflowY: 'auto', flex: 1 }}>
              <div style={{ marginBottom: '14px' }}>
                <label style={{ display: 'block', fontSize: '12px', color: 'var(--muted)', marginBottom: '4px' }}>
                  Skill Identifier Name * (lowercase letters, numbers, and hyphens)
                </label>
                <input
                  type="text"
                  required
                  placeholder="e.g. custom-cloud-recon"
                  pattern="^[a-z0-9]+(-[a-z0-9]+)*$"
                  value={createForm.name}
                  onChange={e => setCreateForm({ ...createForm, name: e.target.value.toLowerCase() })}
                  style={{ width: '100%', boxSizing: 'border-box' }}
                />
                <span style={{ fontSize: '11px', color: 'var(--muted)' }}>
                  Must match the folder name; e.g. custom-cloud-recon
                </span>
              </div>

              <div style={{ marginBottom: '14px' }}>
                <label style={{ display: 'block', fontSize: '12px', color: 'var(--muted)', marginBottom: '4px' }}>
                  Description * (summarize capabilities for the AI model)
                </label>
                <input
                  type="text"
                  required
                  placeholder="Specialized procedures for evaluating..."
                  value={createForm.description}
                  onChange={e => setCreateForm({ ...createForm, description: e.target.value })}
                  style={{ width: '100%', boxSizing: 'border-box' }}
                />
              </div>

              <div style={{ marginBottom: '16px' }}>
                <label style={{ display: 'block', fontSize: '12px', color: 'var(--muted)', marginBottom: '4px' }}>
                  SKILL.md Markdown Body *
                </label>
                <textarea
                  rows={10}
                  required
                  value={createForm.content}
                  onChange={e => setCreateForm({ ...createForm, content: e.target.value })}
                  style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '12px' }}
                />
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  style={{ background: 'var(--surface2)' }}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creatingSkill}
                  className="primary"
                >
                  {creatingSkill ? 'Creating…' : 'Create Skill'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
