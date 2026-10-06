import { useState, useEffect, createContext, useContext } from 'react'
import { BrowserRouter, Routes, Route, Navigate, Link, useLocation, useNavigate } from 'react-router-dom'
import { api } from './api.js'
import LoginPage from './pages/Login.jsx'
import Dashboard from './pages/Dashboard.jsx'
import AgentPage from './pages/Agent.jsx'
import AssetsPage from './pages/Assets.jsx'
import VulnsPage from './pages/Vulns.jsx'
import AuditPage from './pages/Audit.jsx'
import UsersPage from './pages/Users.jsx'
import RolesPage from './pages/Roles.jsx'
import KnowledgePage from './pages/Knowledge.jsx'
import ChangePasswordPage from './pages/ChangePassword.jsx'
import ProjectsPage from './pages/Projects.jsx'
import BatchTasksPage from './pages/BatchTasks.jsx'
import AttackChainPage from './pages/AttackChain.jsx'
import WorkflowsPage from './pages/Workflows.jsx'
import HITLPage from './pages/HITL.jsx'
import ConversationsPage from './pages/Conversations.jsx'

// ── Auth context ────────────────────────────────────────────────────────────
export const AuthCtx = createContext(null)

function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const token = localStorage.getItem('kestrel_token')
    if (!token) { setLoading(false); return }
    api.me().then(d => {
      setUser(d)
      setLoading(false)
    }).catch(() => {
      localStorage.removeItem('kestrel_token')
      setLoading(false)
    })
  }, [])

  const login = async (username, password) => {
    const d = await api.login(username, password)
    localStorage.setItem('kestrel_token', d.token)
    setUser({ id: d.user_id, username: d.username, display_name: d.display_name, must_change_password: d.must_change_password })
    return d
  }

  const logout = async () => {
    await api.logout().catch(() => {})
    localStorage.removeItem('kestrel_token')
    setUser(null)
  }

  return <AuthCtx.Provider value={{ user, setUser, login, logout, loading }}>{children}</AuthCtx.Provider>
}

// ── Protected route ─────────────────────────────────────────────────────────
function Protected({ children }) {
  const { user, loading } = useContext(AuthCtx)
  const location = useLocation()
  if (loading) return <div style={{ padding: 32, color: 'var(--muted)' }}>Loading…</div>
  if (!user) return <Navigate to="/login" state={{ from: location }} replace />
  if (user.must_change_password && location.pathname !== '/change-password')
    return <Navigate to="/change-password" replace />
  return children
}

// ── Sidebar nav ─────────────────────────────────────────────────────────────
const NAV = [
  { to: '/',              label: '⬡  Dashboard' },
  { to: '/projects',      label: '📁 Projects' },
  { to: '/agent',         label: '🤖 Agent' },
  { to: '/conversations', label: '💬 Conversations' },
  { to: '/batch',         label: '⚙  Batch Tasks' },
  { to: '/workflows',     label: '🔀 Workflows' },
  { to: '/hitl',          label: '✋ Approvals' },
  { to: '/assets',        label: '📦 Assets' },
  { to: '/vulns',         label: '🛡  Vulnerabilities' },
  { to: '/knowledge',     label: '📚 Knowledge' },
  { to: '/audit',         label: '📋 Audit Logs' },
  { to: '/users',         label: '👤 Users' },
  { to: '/roles',         label: '🔑 Roles' },
]

function Sidebar() {
  const { user, logout } = useContext(AuthCtx)
  const location = useLocation()
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  return (
    <aside style={{
      width: 220, minHeight: '100vh', background: 'var(--surface)',
      borderRight: '1px solid var(--border)', display: 'flex', flexDirection: 'column',
      position: 'fixed', top: 0, left: 0, bottom: 0, zIndex: 10,
    }}>
      <div style={{ padding: '20px 16px 12px', borderBottom: '1px solid var(--border)' }}>
        <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--accent)' }}>⚔ Kestrel</div>
        <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 2 }}>Under Development</div>
      </div>
      <nav style={{ flex: 1, padding: '8px 0', overflowY: 'auto' }}>
        {NAV.map(({ to, label }) => (
          <Link key={to} to={to} style={{
            display: 'block', padding: '7px 16px', fontSize: 13,
            color: location.pathname === to ? 'var(--accent)' : 'var(--text)',
            background: location.pathname === to ? 'var(--surface2)' : 'transparent',
            textDecoration: 'none', borderLeft: location.pathname === to ? '3px solid var(--accent)' : '3px solid transparent',
            transition: 'background .1s',
          }}>
            {label}
          </Link>
        ))}
      </nav>
      <div style={{ padding: '12px 16px', borderTop: '1px solid var(--border)', fontSize: 12 }}>
        <div style={{ color: 'var(--muted)', marginBottom: 6 }}>{user?.username}</div>
        <button onClick={handleLogout} style={{ width: '100%', fontSize: 12 }}>Sign out</button>
      </div>
    </aside>
  )
}

function Layout({ children }) {
  return (
    <div style={{ display: 'flex' }}>
      <Sidebar />
      <main style={{ marginLeft: 220, flex: 1, padding: 24, minHeight: '100vh' }}>
        {children}
      </main>
    </div>
  )
}

// ── Root App ─────────────────────────────────────────────────────────────────
export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/change-password" element={<Protected><ChangePasswordPage /></Protected>} />
          <Route path="/" element={<Protected><Layout><Dashboard /></Layout></Protected>} />
          <Route path="/agent" element={<Protected><Layout><AgentPage /></Layout></Protected>} />
          <Route path="/assets" element={<Protected><Layout><AssetsPage /></Layout></Protected>} />
          <Route path="/vulns" element={<Protected><Layout><VulnsPage /></Layout></Protected>} />
          <Route path="/audit" element={<Protected><Layout><AuditPage /></Layout></Protected>} />
          <Route path="/users" element={<Protected><Layout><UsersPage /></Layout></Protected>} />
          <Route path="/roles" element={<Protected><Layout><RolesPage /></Layout></Protected>} />
          <Route path="/knowledge" element={<Protected><Layout><KnowledgePage /></Layout></Protected>} />
          <Route path="/projects" element={<Protected><Layout><ProjectsPage /></Layout></Protected>} />
          <Route path="/projects/:id/attack-chain" element={<Protected><Layout><AttackChainPage /></Layout></Protected>} />
          <Route path="/batch" element={<Protected><Layout><BatchTasksPage /></Layout></Protected>} />
          <Route path="/workflows" element={<Protected><Layout><WorkflowsPage /></Layout></Protected>} />
          <Route path="/hitl" element={<Protected><Layout><HITLPage /></Layout></Protected>} />
          <Route path="/conversations" element={<Protected><Layout><ConversationsPage /></Layout></Protected>} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  )
}
