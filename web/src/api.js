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

  // Agent sessions
  listSessions:        (q)       => request('GET',   `/sessions?${new URLSearchParams(q || {})}`),
  getSession:          (id)      => request('GET',   `/sessions/${id}`),
  updateSession:       (id, d)   => request('PATCH', `/sessions/${id}`, d),
  deleteSession:       (id)      => request('DELETE',`/sessions/${id}`),

  // Tool executions
  listToolExecutions:  (q)       => request('GET',   `/tool-executions?${new URLSearchParams(q || {})}`),

  // Projects
  listProjects:        (q)           => request('GET',    `/projects?${new URLSearchParams(q || {})}`),
  createProject:       (d)           => request('POST',   '/projects', d),
  getProject:          (id)          => request('GET',    `/projects/${id}`),
  updateProject:       (id, d)       => request('PATCH',  `/projects/${id}`, d),
  deleteProject:       (id)          => request('DELETE', `/projects/${id}`),
  listProjectFacts:    (id)          => request('GET',    `/projects/${id}/facts`),
  upsertProjectFact:   (id, d)       => request('POST',   `/projects/${id}/facts`, d),
  deleteProjectFact:   (pid, fid)    => request('DELETE', `/projects/${pid}/facts/${fid}`),
  getAttackChain:      (id)          => request('GET',    `/projects/${id}/attack-chain`),
  addChainNode:        (id, d)       => request('POST',   `/projects/${id}/attack-chain/nodes`, d),
  addChainEdge:        (id, d)       => request('POST',   `/projects/${id}/attack-chain/edges`, d),

  // Batch task queues
  listQueues:          (q)           => request('GET',    `/batch/queues?${new URLSearchParams(q || {})}`),
  createQueue:         (d)           => request('POST',   '/batch/queues', d),
  getQueue:            (id)          => request('GET',    `/batch/queues/${id}`),
  runQueue:            (id)          => request('POST',   `/batch/queues/${id}/run`),
  cancelQueue:         (id)          => request('POST',   `/batch/queues/${id}/cancel`),
  deleteQueue:         (id)          => request('DELETE', `/batch/queues/${id}`),

  // HITL approvals
  listPendingHITL:     ()            => request('GET',    '/hitl/pending'),
  decideHITL:          (id, d)       => request('POST',   `/hitl/${id}/decide`, d),

  // Conversations
  listConversations:   (q)           => request('GET',    `/conversations?${new URLSearchParams(q || {})}`),
  createConversation:  (d)           => request('POST',   '/conversations', d),
  updateConversation:  (id, d)       => request('PATCH',  `/conversations/${id}`, d),
  deleteConversation:  (id)          => request('DELETE', `/conversations/${id}`),
  getConvMessages:     (id)          => request('GET',    `/conversations/${id}/messages`),

  // MCP servers
  listMCPServers:      ()            => request('GET',    '/mcp/servers'),
  upsertMCPServer:     (d)           => request('POST',   '/mcp/servers', d),
  deleteMCPServer:     (id)          => request('DELETE', `/mcp/servers/${id}`),
  resetCircuit:        (id)          => request('POST',   `/mcp/servers/${id}/reset-circuit`),

  // Workflows
  listWorkflows:       ()            => request('GET',    '/workflows'),
  createWorkflow:      (d)           => request('POST',   '/workflows', d),
  getWorkflow:         (id)          => request('GET',    `/workflows/${id}`),
  updateWorkflow:      (id, d)       => request('PATCH',  `/workflows/${id}`, d),
  deleteWorkflow:      (id)          => request('DELETE', `/workflows/${id}`),
  runWorkflow:         (id, d)       => request('POST',   `/workflows/${id}/run`, d),
  getWorkflowRun:      (runId)       => request('GET',    `/workflows/runs/${runId}`),
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
