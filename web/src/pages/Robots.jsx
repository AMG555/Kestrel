import React, { useState, useEffect } from 'react'
import { api } from '../api'

export default function Robots() {
  const [config, setConfig] = useState({
    enabled: true,
    webhook_url: '',
    slack_webhook_url: '',
    discord_webhook_url: '',
    telegram_bot_token: '',
    telegram_chat_id: '',
    notify_on_critical_vuln: true,
    notify_on_hitl: true,
    notify_on_task_done: true,
  })
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testStatus, setTestStatus] = useState({})
  const [notice, setNotice] = useState(null)

  useEffect(() => {
    loadConfig()
  }, [])

  async function loadConfig() {
    setLoading(true)
    try {
      const data = await api.getRobotsConfig()
      if (data) setConfig(data)
    } catch (err) {
      setNotice({ type: 'error', text: `Failed to load config: ${err.message}` })
    } finally {
      setLoading(false)
    }
  }

  async function handleSave(e) {
    e.preventDefault()
    setSaving(true)
    setNotice(null)
    try {
      await api.updateRobotsConfig(config)
      setNotice({ type: 'success', text: 'Notification bot settings saved successfully!' })
    } catch (err) {
      setNotice({ type: 'error', text: `Failed to save: ${err.message}` })
    } finally {
      setSaving(false)
    }
  }

  async function handleTest(channel) {
    setTestStatus(prev => ({ ...prev, [channel]: 'sending' }))
    try {
      await api.testRobot(channel)
      setTestStatus(prev => ({ ...prev, [channel]: 'success' }))
      setTimeout(() => {
        setTestStatus(prev => ({ ...prev, [channel]: null }))
      }, 3000)
    } catch (err) {
      setTestStatus(prev => ({ ...prev, [channel]: `Error: ${err.message}` }))
    }
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ marginBottom: '24px' }}>
        <h1 style={{ margin: 0, fontSize: '26px', fontWeight: 700, display: 'flex', alignItems: 'center', gap: '10px' }}>
          <span>🤖</span> Notification Bots & Webhooks
        </h1>
        <p style={{ margin: '6px 0 0', color: '#94a3b8', fontSize: '14px' }}>
          Real-time proactive security alerting to Slack, Discord, Telegram, and generic Webhooks for critical findings and HITL approvals.
        </p>
      </div>

      {notice && (
        <div style={{
          padding: '12px 16px',
          borderRadius: '8px',
          marginBottom: '20px',
          background: notice.type === 'error' ? '#7f1d1d33' : '#064e3b33',
          border: `1px solid ${notice.type === 'error' ? '#dc2626' : '#059669'}`,
          color: notice.type === 'error' ? '#fca5a5' : '#6ee7b7',
          fontSize: '14px'
        }}>
          {notice.text}
        </div>
      )}

      {loading ? (
        <div style={{ padding: '60px', textAlign: 'center', color: '#64748b' }}>Loading notification settings...</div>
      ) : (
        <form onSubmit={handleSave}>
          {/* Global Alert Trigger Policies */}
          <div style={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: '10px', padding: '20px', marginBottom: '24px' }}>
            <h2 style={{ margin: '0 0 16px', fontSize: '16px', color: '#f8fafc', fontWeight: 600 }}>
              Trigger Policy
            </h2>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '16px' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: '10px', color: '#e2e8f0', cursor: 'pointer', fontSize: '13px' }}>
                <input
                  type="checkbox"
                  checked={config.enabled}
                  onChange={e => setConfig({ ...config, enabled: e.target.checked })}
                  style={{ width: '16px', height: '16px', accentColor: '#2563eb' }}
                />
                Enable Global Dispatch
              </label>

              <label style={{ display: 'flex', alignItems: 'center', gap: '10px', color: '#e2e8f0', cursor: 'pointer', fontSize: '13px' }}>
                <input
                  type="checkbox"
                  checked={config.notify_on_critical_vuln}
                  onChange={e => setConfig({ ...config, notify_on_critical_vuln: e.target.checked })}
                  style={{ width: '16px', height: '16px', accentColor: '#2563eb' }}
                />
                Critical Findings
              </label>

              <label style={{ display: 'flex', alignItems: 'center', gap: '10px', color: '#e2e8f0', cursor: 'pointer', fontSize: '13px' }}>
                <input
                  type="checkbox"
                  checked={config.notify_on_hitl}
                  onChange={e => setConfig({ ...config, notify_on_hitl: e.target.checked })}
                  style={{ width: '16px', height: '16px', accentColor: '#2563eb' }}
                />
                HITL Approval Requests
              </label>

              <label style={{ display: 'flex', alignItems: 'center', gap: '10px', color: '#e2e8f0', cursor: 'pointer', fontSize: '13px' }}>
                <input
                  type="checkbox"
                  checked={config.notify_on_task_done}
                  onChange={e => setConfig({ ...config, notify_on_task_done: e.target.checked })}
                  style={{ width: '16px', height: '16px', accentColor: '#2563eb' }}
                />
                Task/Workflow Completion
              </label>
            </div>
          </div>

          {/* Platform Configuration Cards */}
          <div style={{ display: 'grid', gap: '20px', marginBottom: '24px' }}>
            {/* Generic Webhook */}
            <div style={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: '10px', padding: '20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '14px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                  <span style={{ fontSize: '20px' }}>🌐</span>
                  <h3 style={{ margin: 0, fontSize: '16px', color: '#f8fafc' }}>Generic Webhook (HTTP POST)</h3>
                </div>
                <button
                  type="button"
                  onClick={() => handleTest('webhook')}
                  disabled={!config.webhook_url || testStatus.webhook === 'sending'}
                  style={{
                    background: '#1e293b',
                    color: '#38bdf8',
                    border: '1px solid #334155',
                    padding: '6px 12px',
                    borderRadius: '6px',
                    cursor: config.webhook_url ? 'pointer' : 'not-allowed',
                    fontSize: '12px',
                    fontWeight: 500
                  }}
                >
                  {testStatus.webhook === 'sending' ? 'Sending...' : testStatus.webhook === 'success' ? '✓ Sent!' : 'Send Test Alert'}
                </button>
              </div>
              <input
                type="text"
                placeholder="https://example.com/api/security-webhook"
                value={config.webhook_url || ''}
                onChange={e => setConfig({ ...config, webhook_url: e.target.value })}
                style={{
                  width: '100%',
                  boxSizing: 'border-box',
                  background: '#1e293b',
                  border: '1px solid #334155',
                  borderRadius: '6px',
                  padding: '10px 14px',
                  color: '#f8fafc',
                  fontSize: '13px'
                }}
              />
              {testStatus.webhook && testStatus.webhook !== 'sending' && testStatus.webhook !== 'success' && (
                <div style={{ color: '#f87171', fontSize: '12px', marginTop: '6px' }}>{testStatus.webhook}</div>
              )}
            </div>

            {/* Slack */}
            <div style={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: '10px', padding: '20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '14px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                  <span style={{ fontSize: '20px' }}>💬</span>
                  <h3 style={{ margin: 0, fontSize: '16px', color: '#f8fafc' }}>Slack Incoming Webhook</h3>
                </div>
                <button
                  type="button"
                  onClick={() => handleTest('slack')}
                  disabled={!config.slack_webhook_url || testStatus.slack === 'sending'}
                  style={{
                    background: '#1e293b',
                    color: '#38bdf8',
                    border: '1px solid #334155',
                    padding: '6px 12px',
                    borderRadius: '6px',
                    cursor: config.slack_webhook_url ? 'pointer' : 'not-allowed',
                    fontSize: '12px',
                    fontWeight: 500
                  }}
                >
                  {testStatus.slack === 'sending' ? 'Sending...' : testStatus.slack === 'success' ? '✓ Sent!' : 'Send Test Alert'}
                </button>
              </div>
              <input
                type="text"
                placeholder="https://hooks.slack.com/services/T000/B000/XXXX"
                value={config.slack_webhook_url || ''}
                onChange={e => setConfig({ ...config, slack_webhook_url: e.target.value })}
                style={{
                  width: '100%',
                  boxSizing: 'border-box',
                  background: '#1e293b',
                  border: '1px solid #334155',
                  borderRadius: '6px',
                  padding: '10px 14px',
                  color: '#f8fafc',
                  fontSize: '13px'
                }}
              />
              {testStatus.slack && testStatus.slack !== 'sending' && testStatus.slack !== 'success' && (
                <div style={{ color: '#f87171', fontSize: '12px', marginTop: '6px' }}>{testStatus.slack}</div>
              )}
            </div>

            {/* Discord */}
            <div style={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: '10px', padding: '20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '14px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                  <span style={{ fontSize: '20px' }}>🎮</span>
                  <h3 style={{ margin: 0, fontSize: '16px', color: '#f8fafc' }}>Discord Webhook</h3>
                </div>
                <button
                  type="button"
                  onClick={() => handleTest('discord')}
                  disabled={!config.discord_webhook_url || testStatus.discord === 'sending'}
                  style={{
                    background: '#1e293b',
                    color: '#38bdf8',
                    border: '1px solid #334155',
                    padding: '6px 12px',
                    borderRadius: '6px',
                    cursor: config.discord_webhook_url ? 'pointer' : 'not-allowed',
                    fontSize: '12px',
                    fontWeight: 500
                  }}
                >
                  {testStatus.discord === 'sending' ? 'Sending...' : testStatus.discord === 'success' ? '✓ Sent!' : 'Send Test Alert'}
                </button>
              </div>
              <input
                type="text"
                placeholder="https://discord.com/api/webhooks/0000000000/XXXXXX"
                value={config.discord_webhook_url || ''}
                onChange={e => setConfig({ ...config, discord_webhook_url: e.target.value })}
                style={{
                  width: '100%',
                  boxSizing: 'border-box',
                  background: '#1e293b',
                  border: '1px solid #334155',
                  borderRadius: '6px',
                  padding: '10px 14px',
                  color: '#f8fafc',
                  fontSize: '13px'
                }}
              />
              {testStatus.discord && testStatus.discord !== 'sending' && testStatus.discord !== 'success' && (
                <div style={{ color: '#f87171', fontSize: '12px', marginTop: '6px' }}>{testStatus.discord}</div>
              )}
            </div>

            {/* Telegram */}
            <div style={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: '10px', padding: '20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '14px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                  <span style={{ fontSize: '20px' }}>✈️</span>
                  <h3 style={{ margin: 0, fontSize: '16px', color: '#f8fafc' }}>Telegram Bot</h3>
                </div>
                <button
                  type="button"
                  onClick={() => handleTest('telegram')}
                  disabled={!config.telegram_bot_token || !config.telegram_chat_id || testStatus.telegram === 'sending'}
                  style={{
                    background: '#1e293b',
                    color: '#38bdf8',
                    border: '1px solid #334155',
                    padding: '6px 12px',
                    borderRadius: '6px',
                    cursor: (config.telegram_bot_token && config.telegram_chat_id) ? 'pointer' : 'not-allowed',
                    fontSize: '12px',
                    fontWeight: 500
                  }}
                >
                  {testStatus.telegram === 'sending' ? 'Sending...' : testStatus.telegram === 'success' ? '✓ Sent!' : 'Send Test Alert'}
                </button>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
                <div>
                  <label style={{ display: 'block', fontSize: '12px', color: '#94a3b8', marginBottom: '4px' }}>Bot Token</label>
                  <input
                    type="password"
                    placeholder="123456789:ABCDefGhIJKlmNoPQRsTUVwxyZ"
                    value={config.telegram_bot_token || ''}
                    onChange={e => setConfig({ ...config, telegram_bot_token: e.target.value })}
                    style={{
                      width: '100%',
                      boxSizing: 'border-box',
                      background: '#1e293b',
                      border: '1px solid #334155',
                      borderRadius: '6px',
                      padding: '10px 14px',
                      color: '#f8fafc',
                      fontSize: '13px'
                    }}
                  />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: '12px', color: '#94a3b8', marginBottom: '4px' }}>Chat ID / Channel</label>
                  <input
                    type="text"
                    placeholder="-100123456789"
                    value={config.telegram_chat_id || ''}
                    onChange={e => setConfig({ ...config, telegram_chat_id: e.target.value })}
                    style={{
                      width: '100%',
                      boxSizing: 'border-box',
                      background: '#1e293b',
                      border: '1px solid #334155',
                      borderRadius: '6px',
                      padding: '10px 14px',
                      color: '#f8fafc',
                      fontSize: '13px'
                    }}
                  />
                </div>
              </div>
              {testStatus.telegram && testStatus.telegram !== 'sending' && testStatus.telegram !== 'success' && (
                <div style={{ color: '#f87171', fontSize: '12px', marginTop: '6px' }}>{testStatus.telegram}</div>
              )}
            </div>
          </div>

          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <button
              type="submit"
              disabled={saving}
              style={{
                background: '#2563eb',
                color: '#ffffff',
                border: 'none',
                padding: '10px 24px',
                borderRadius: '6px',
                cursor: saving ? 'not-allowed' : 'pointer',
                fontWeight: 600,
                fontSize: '14px'
              }}
            >
              {saving ? 'Saving...' : 'Save Configuration'}
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
