import { Loader2 } from 'lucide-react'
import { lazy, Suspense, type ReactNode } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router'

import { Layout } from '@/components/Layout'
import { RequireAuth } from '@/components/RequireAuth'
import { Dashboard } from '@/pages/Dashboard'
import { Login } from '@/pages/Login'

// Unterseiten erst bei Bedarf laden (kleineres Start-Bundle).
const Settings = lazy(() => import('@/pages/Settings').then((m) => ({ default: m.Settings })))
const Backups = lazy(() => import('@/pages/Backups').then((m) => ({ default: m.Backups })))
const Checks = lazy(() => import('@/pages/Checks').then((m) => ({ default: m.Checks })))
const History = lazy(() => import('@/pages/History').then((m) => ({ default: m.History })))
const Systems = lazy(() => import('@/pages/Systems').then((m) => ({ default: m.Systems })))
const SystemDetail = lazy(() => import('@/pages/SystemDetail').then((m) => ({ default: m.SystemDetail })))

function Lazy({ children }: { children: ReactNode }) {
  return (
    <Suspense
      fallback={
        <div className="grid place-items-center py-24">
          <Loader2 className="text-muted-foreground size-6 animate-spin" />
        </div>
      }
    >
      {children}
    </Suspense>
  )
}

/**
 * Alte Adressen der Seiten „Dienste“ und „Ressourcen“ (Lesezeichen, Links in
 * Benachrichtigungen): /dienste#integration-3 → /systeme/3, sonst → /systeme.
 */
function LegacySystems() {
  const m = /^#integration-(\d+)$/.exec(useLocation().hash)
  return <Navigate to={m ? `/systeme/${m[1]}` : '/systeme'} replace />
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Dashboard />} />
        <Route path="d/:id" element={<Dashboard />} />
        <Route path="backups" element={<Lazy><Backups /></Lazy>} />
        <Route path="checks" element={<Lazy><Checks /></Lazy>} />
        <Route path="systeme" element={<Lazy><Systems /></Lazy>} />
        <Route path="systeme/:id" element={<Lazy><SystemDetail /></Lazy>} />
        <Route path="dienste" element={<LegacySystems />} />
        <Route path="ressourcen" element={<LegacySystems />} />
        <Route path="verlauf" element={<Lazy><History /></Lazy>} />
        <Route path="einstellungen" element={<Lazy><Settings /></Lazy>} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
