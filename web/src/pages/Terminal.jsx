import React, { useState, useRef, useEffect } from 'react'
import { api } from '../api'

const PRESET_COMMANDS = [
  { label: 'Network Scan', cmd: 'nmap -sV -T4 127.0.0.1' },
  { label: 'Subdomain Enum', cmd: 'subfinder -d example.com -silent' },
  { label: 'HTTP Prober', cmd: 'httpx -u https://example.com -status-code -title' },
  { label: 'Nuclei Scan', cmd: 'nuclei -u https://example.com -severity critical,high' },
  { label: 'Dirsearch', cmd: 'dirsearch -u https://example.com -e php,html,js' },
  { label: 'SQLMap Check', cmd: 'sqlmap -u "https://example.com?id=1" --batch' },
]

export default function Terminal() {
  const [command, setCommand] = useState('')
  const [cwd, setCwd] = useState('')
  const [timeout, setTimeoutSec] = useState(30)
  const [history, setHistory] = useState([
    {
      id: 'init',
      cmd: '# Kestrel Authorized Security Operator Shell',
      stdout: 'System initialized. All commands are audited to the immutable security log.\nRestricted to authorized target scopes only.',
      stderr: '',
      exitCode: 0,
      durationMs: 0,
      time: new Date().toLocaleTimeString(),
    }
  ])
  const [running, setRunning] = useState(false)
  const terminalEndRef = useRef(null)

  useEffect(() => {
    terminalEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [history])

  async function handleRun(e) {
    if (e) e.preventDefault()
    const trimmed = command.trim()
    if (!trimmed || running) return

    setRunning(true)
    const runTime = new Date().toLocaleTimeString()
    try {
      const res = await api.execTerminal({
        command: trimmed,
        cwd: cwd.trim() || undefined,
        timeout: parseInt(timeout, 10) || 30,
      })

      setHistory(prev => [
        ...prev,
        {
          id: Date.now(),
          cmd: trimmed,
          stdout: res.stdout,
          stderr: res.stderr,
          exitCode: res.exit_code,
          durationMs: res.duration_ms,
          error: res.error,
          time: runTime,
        }
      ])
      setCommand('')
    } catch (err) {
      setHistory(prev => [
        ...prev,
        {
          id: Date.now(),
          cmd: trimmed,
          stdout: '',
          stderr: err.message,
          exitCode: 1,
          durationMs: 0,
          error: err.message,
          time: runTime,
        }
      ])
    } finally {
      setRunning(false)
    }
  }

  function handleClear() {
    setHistory([])
  }

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto', display: 'flex', flexDirection: 'column', height: 'calc(100vh - 48px)' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0, display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>💻</span> Operator Terminal
          </h1>
          <p style={{ color: 'var(--muted)', fontSize: 13, margin: '4px 0 0' }}>
            Interactive authenticated command shell with audit logging and timeout isolation.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <button onClick={handleClear} style={{ fontSize: 12, padding: '4px 10px' }}>
            Clear Screen
          </button>
        </div>
      </div>

      {/* Preset pills */}
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 12 }}>
        <span style={{ fontSize: 12, color: 'var(--muted)', alignSelf: 'center' }}>Presets:</span>
        {PRESET_COMMANDS.map((p) => (
          <button
            key={p.label}
            onClick={() => setCommand(p.cmd)}
            style={{ fontSize: 12, padding: '3px 8px', background: 'var(--surface)', border: '1px solid var(--border)' }}
          >
            {p.label}
          </button>
        ))}
      </div>

      {/* Terminal Viewport */}
      <div style={{
        flex: 1,
        background: '#090d13',
        border: '1px solid var(--border)',
        borderRadius: 8,
        padding: 16,
        overflowY: 'auto',
        fontFamily: 'Consolas, Monaco, "Courier New", monospace',
        fontSize: 13,
        display: 'flex',
        flexDirection: 'column',
        gap: 16,
      }}>
        {history.map((item) => (
          <div key={item.id} style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {/* Prompt Line */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: 'var(--accent)' }}>
              <span style={{ color: 'var(--success)', fontWeight: 700 }}>kestrel@ops:~$</span>
              <span style={{ color: '#fff', fontWeight: 600 }}>{item.cmd}</span>
              <div style={{ marginLeft: 'auto', display: 'flex', gap: 8, fontSize: 11, color: 'var(--muted)' }}>
                {item.durationMs > 0 && <span>{item.durationMs}ms</span>}
                {item.exitCode !== undefined && (
                  <span style={{
                    padding: '1px 6px',
                    borderRadius: 4,
                    background: item.exitCode === 0 ? 'rgba(63,185,80,0.15)' : 'rgba(248,81,73,0.15)',
                    color: item.exitCode === 0 ? 'var(--success)' : 'var(--danger)',
                    border: `1px solid ${item.exitCode === 0 ? 'var(--success)' : 'var(--danger)'}`,
                  }}>
                    exit {item.exitCode}
                  </span>
                )}
                <span>{item.time}</span>
              </div>
            </div>

            {/* Stdout Output */}
            {item.stdout && (
              <pre style={{
                margin: 0,
                color: '#c9d1d9',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                background: 'rgba(255,255,255,0.02)',
                padding: '8px 12px',
                borderRadius: 4,
                lineHeight: 1.45,
              }}>
                {item.stdout}
              </pre>
            )}

            {/* Stderr Output */}
            {item.stderr && (
              <pre style={{
                margin: 0,
                color: 'var(--danger)',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                background: 'rgba(248,81,73,0.05)',
                padding: '8px 12px',
                borderRadius: 4,
                borderLeft: '2px solid var(--danger)',
                lineHeight: 1.45,
              }}>
                {item.stderr}
              </pre>
            )}

            {/* Error banner */}
            {item.error && !item.stderr && (
              <div style={{ color: 'var(--danger)', fontSize: 12 }}>
                ⚠️ Error: {item.error}
              </div>
            )}
          </div>
        ))}
        {running && (
          <div style={{ color: 'var(--accent)', display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>⏳ Executing command on host...</span>
          </div>
        )}
        <div ref={terminalEndRef} />
      </div>

      {/* Input Bar */}
      <form onSubmit={handleRun} style={{ marginTop: 12, display: 'flex', gap: 8, alignItems: 'center' }}>
        <input
          type="text"
          placeholder="Working directory (optional, e.g. /opt/tools)"
          value={cwd}
          onChange={(e) => setCwd(e.target.value)}
          style={{ width: 220, fontSize: 12, padding: '8px 10px' }}
        />
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          <span style={{ fontSize: 12, color: 'var(--muted)' }}>Timeout:</span>
          <select
            value={timeout}
            onChange={(e) => setTimeoutSec(e.target.value)}
            style={{ fontSize: 12, padding: '7px 8px' }}
          >
            <option value="15">15s</option>
            <option value="30">30s</option>
            <option value="60">60s</option>
            <option value="120">120s</option>
          </select>
        </div>
        <div style={{ flex: 1, position: 'relative' }}>
          <input
            type="text"
            placeholder="Type command and press Enter (e.g. whoami, nmap -sn 192.168.1.0/24)..."
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            disabled={running}
            style={{
              width: '100%',
              fontSize: 13,
              fontFamily: 'Consolas, monospace',
              padding: '8px 12px',
            }}
          />
        </div>
        <button
          type="submit"
          className="primary"
          disabled={running || !command.trim()}
          style={{ padding: '8px 18px', minWidth: 90 }}
        >
          {running ? 'Running…' : 'Execute'}
        </button>
      </form>
    </div>
  )
}
