import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api, integrationApi, type IntegrationInput, type Settings } from './api'
import { useLive } from './live-context'
import { keys } from './queries-keys'

export { keys }

// Ohne Live-Verbindung (SSE) wird regelmäßig nachgeladen; mit ihr nur noch
// selten als Absicherung.
const POLL_MS = 30_000
const LIVE_POLL_MS = 5 * 60_000

function usePollInterval() {
  return useLive() ? LIVE_POLL_MS : POLL_MS
}

export function useSession() {
  return useQuery({ queryKey: keys.session, queryFn: api.session, staleTime: 60_000 })
}

export function useSettings() {
  return useQuery({ queryKey: keys.settings, queryFn: api.settings })
}

export function useSaveSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (s: Settings) => api.saveSettings(s),
    onSuccess: (s) => qc.setQueryData(keys.settings, s),
  })
}

export function useInfo() {
  return useQuery({ queryKey: keys.info, queryFn: api.info, staleTime: Infinity })
}

export function useIntegrationTypes() {
  return useQuery({ queryKey: keys.integrationTypes, queryFn: integrationApi.types, staleTime: 5 * 60_000 })
}

export function useIntegrations() {
  const poll = usePollInterval()
  return useQuery({ queryKey: keys.integrations, queryFn: integrationApi.list, refetchInterval: poll })
}

export function useSaveIntegration() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id?: number; input: IntegrationInput }) =>
      id ? integrationApi.update(id, input) : integrationApi.create(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.integrations }),
  })
}

export function useDeleteIntegration() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: integrationApi.remove,
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.integrations }),
  })
}

export function useTestIntegration() {
  return useMutation({
    mutationFn: ({ id, input }: { id?: number; input: IntegrationInput }) => integrationApi.test(input, id),
  })
}

export function usePollIntegration() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: integrationApi.poll,
    onSettled: () => qc.invalidateQueries({ queryKey: keys.integrations }),
  })
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.login,
    onSuccess: (session) => qc.setQueryData(keys.session, session),
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.logout,
    onSettled: () => {
      // Kein qc.clear(): das entfernt auch die Session-Query, ohne ihre
      // Beobachter zu benachrichtigen – RequireAuth bekäme den Logout nie mit.
      qc.setQueryData(keys.session, { authenticated: false })
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== keys.session[0] })
    },
  })
}
