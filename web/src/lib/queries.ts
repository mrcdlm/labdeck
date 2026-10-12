import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import {
  api,
  dashboardApi,
  historyApi,
  integrationApi,
  monitoringApi,
  webhookApi,
  type Dashboard,
  type HistoryFilter,
  type IntegrationInput,
  type MetricRange,
  type Settings,
  type WebhookInput,
} from './api'
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
    onSuccess: (s) => {
      qc.setQueryData(keys.settings, s)
      // Standard-Höchstalter der Backups
      void qc.invalidateQueries({ queryKey: keys.backups })
      void qc.invalidateQueries({ queryKey: keys.overview })
    },
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

export function useOverview() {
  const poll = usePollInterval()
  return useQuery({ queryKey: keys.overview, queryFn: monitoringApi.overview, refetchInterval: poll })
}

export function useChecks() {
  const poll = usePollInterval()
  return useQuery({ queryKey: keys.checks, queryFn: monitoringApi.checks, refetchInterval: poll })
}

// Backups altern auch ohne neue Daten (bald fällig → überfällig): regelmäßig neu auswerten.
export function useBackups() {
  return useQuery({ queryKey: keys.backups, queryFn: monitoringApi.backups, refetchInterval: POLL_MS })
}

export function useSetBackupMaxAge() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ integrationId, itemId, hours }: { integrationId: number; itemId: string; hours: number }) =>
      monitoringApi.setBackupMaxAge(integrationId, itemId, hours),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.backups })
      void qc.invalidateQueries({ queryKey: keys.overview })
    },
  })
}

export function useMetricHistory(id: number, key: string | undefined, range: MetricRange) {
  return useQuery({
    queryKey: [...keys.history, id, key, range],
    queryFn: () => integrationApi.history(id, key!, range),
    enabled: key !== undefined,
    placeholderData: (prev) => (prev?.key === key ? prev : undefined),
  })
}

export function useWebhooks() {
  return useQuery({ queryKey: keys.webhooks, queryFn: webhookApi.list })
}

export function useSaveWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id?: number; input: WebhookInput }) =>
      id ? webhookApi.update(id, input) : webhookApi.create(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.webhooks }),
  })
}

export function useDeleteWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: webhookApi.remove,
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.webhooks }),
  })
}

export function useTestWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: webhookApi.test,
    onSettled: () => qc.invalidateQueries({ queryKey: keys.webhooks }),
  })
}

/** Verlauf seitenweise (ältere per fetchNextPage). */
export function useHistory(filter: HistoryFilter, limit = 50) {
  return useInfiniteQuery({
    queryKey: [...keys.events, filter, limit],
    queryFn: ({ pageParam }) => historyApi.list(filter, pageParam, limit),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => (last.more ? last.entries[last.entries.length - 1]?.id : undefined),
  })
}

export function useDashboards() {
  return useQuery({ queryKey: keys.dashboard, queryFn: dashboardApi.list, select: (r) => r.dashboards })
}

/** Ändert Dashboards; die Antwort ist immer die vollständige Liste. */
function useDashboardMutation<T>(fn: (arg: T) => Promise<{ dashboards: Dashboard[] }>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: (r) => qc.setQueryData(keys.dashboard, r),
  })
}

export const useSaveDashboard = () => useDashboardMutation((d: Dashboard) => dashboardApi.save(d))
export const useCreateDashboard = () =>
  useDashboardMutation((d: { name: string; icon: string }) => dashboardApi.create(d.name, d.icon))
export const useDeleteDashboard = () => useDashboardMutation((id: string) => dashboardApi.remove(id))
export const useOrderDashboards = () => useDashboardMutation((ids: string[]) => dashboardApi.order(ids))
