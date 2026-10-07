import { Loader2 } from 'lucide-react'
import { lazy, Suspense, type ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router'

import { Layout } from '@/components/Layout'
import { RequireAuth } from '@/components/RequireAuth'
import { Dashboard } from '@/pages/Dashboard'
import { Login } from '@/pages/Login'

// Unterseiten erst bei Bedarf laden (kleineres Start-Bundle).
const Settings = lazy(() => import('@/pages/Settings').then((m) => ({ default: m.Settings })))
const Backups = lazy(() => import('@/pages/Backups').then((m) => ({ default: m.Backups })))
const Checks = lazy(() => import('@/pages/Checks').then((m) => ({ default: m.Checks })))
const History = lazy(() => import('@/pages/History').then((m) => ({ default: m.History })))
const Resources = lazy(() => import('@/pages/Resources').then((m) => ({ default: m.Resources })))
const Services = lazy(() => import('@/pages/Services').then((m) => ({ default: m.Services })))

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
        <Route path="backups" element={<Lazy><Backups /></Lazy>} />
        <Route path="checks" element={<Lazy><Checks /></Lazy>} />
        <Route path="dienste" element={<Lazy><Services /></Lazy>} />
        <Route path="ressourcen" element={<Lazy><Resources /></Lazy>} />
        <Route path="verlauf" element={<Lazy><History /></Lazy>} />
        <Route path="einstellungen" element={<Lazy><Settings /></Lazy>} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
