import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api, type Settings } from './api'
import { keys } from './queries-keys'

export { keys }

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
