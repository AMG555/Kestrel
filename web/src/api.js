// Centralised API client — wraps fetch with auth header injection.
const BASE = '/api'

function getToken() {
  return localStorage.getItem('kestrel_token') || ''
}

async function request(method, path, body) {
  const headers = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    localStorage.removeItem('kestrel_token')
    window.location.href = '/login'
    return
  }

  const text = await res.text()
  let data
  try { data = JSON.parse(text) } catch { data = { message: text } }

  if (!res.ok) {
    throw new Error(data.error || data.message || `HTTP ${res.status}`)
  }
  return data
}

export const api = {
  login:          (u, p)  => request('POST', '/auth/login', { username: u, password: p }),
  logout:         ()      => request('POST', '/auth/logout'),
  me:             ()      => request('GET',  '/auth/me'),
  changePassword: (cur, nw) => request('POST', '/auth/change-password', { current_password: cur, new_password: nw }),

  // Dashboard
  stats:          ()      => request('GET',  '/dashboard/stats'),
  systemInfo:     ()      => request('GET',  '/system/info'),

  // Users
  listUsers:      ()      => request('GET',  '/users'),
  createUser:     (d)     => request('POST', '/users', d),
  updateUser:     (id, d) => request('PATCH', `/users/${id}`, d),
  deleteUser:     (id)    => request('DELETE', `/users/${id}`),
  assignRole:     (uid, rid) => request('POST', `/users/${uid}/roles`, { role_id: rid }),
  revokeRole:     (uid, rid) => request('DELETE', `/users/${uid}/roles/${rid}`),

  // Roles
  listRoles:      ()      => request('GET',  '/roles'),
  createRole:     (d)     => request('POST', '/roles', d),
  updateRole:     (id, d) => request('PATCH', `/roles/${id}`, d),
  deleteRole:     (id)    => request('DELETE', `/roles/${id}`),

  // Assets
  listAssets:     (q)     => request('GET',  `/assets?${new URLSearchParams(q || {})}`),
  createAsset:    (d)     => request('POST', '/assets', d),
  deleteAsset:    (id)    => request('DELETE', `/assets/${id}`),

  // Vulnerabilities
  listVulns:      (q)     => request('GET',  `/vulnerabilities?${new URLSearchParams(q || {})}`),
  createVuln:     (d)     => request('POST', '/vulnerabilities', d),
  updateVuln:     (id, d) => request('PATCH', `/vulnerabilities/${id}`, d),
  deleteVuln:     (id)    => request('DELETE', `/vulnerabilities/${id}`),

  // Tools
  listTools:      ()      => request('GET',  '/tools'),
  executeTool:    (name, args) => request('POST', `/tools/${name}/execute`, { arguments: args }),

  // Audit
  listAudit:      (q)     => request('GET',  `/audit?${new URLSearchParams(q || {})}`),

  // Agent
  runAgent:       (d)     => request('POST', '/agent/run', d),
}

// WebSocket helper for streaming agent execution.
export function connectAgentStream(onEvent, onClose) {
  const token = getToken()
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const ws = new WebSocket(`${proto}://${window.location.host}/api/agent/stream?token=${token}`)
  ws.onmessage = (e) => {
    try { onEvent(JSON.parse(e.data)) } catch { /* ignore */ }
  }
  ws.onclose = onClose
  ws.onerror = onClose
  return ws
}
