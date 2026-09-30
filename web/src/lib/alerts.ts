import { z } from 'zod'
import { queryOptions } from '@tanstack/react-query'
import { ApiError, request } from './request'

const alertSchema = z.object({
  quota_threshold: z.number().int().min(0).max(99).default(0),
  model_alerts: z.boolean().default(false),
  enabled: z.boolean(),
  configured: z.boolean(),
  destination: z.string().max(300),
  last_delivered_at: z.number().int().nonnegative(),
  next_retry_at: z.number().int().nonnegative(),
  delivery_failed: z.boolean(),
  incidents: z
    .array(
      z.object({
        kind: z.enum([
          'account_reauthorization',
          'pool_unavailable',
          'request_failures',
          'model_unavailable',
          'quota_low',
        ]),
        subject: z.string().max(160),
        since: z.number().int(),
      }),
    )
    .max(8192),
})
export type AlertState = z.infer<typeof alertSchema>
export type AlertInput = {
  enabled: boolean
  url: string
  clear: boolean
  quota_threshold: number
  model_alerts: boolean
}
export const testAlerts = (_input: void, signal: AbortSignal) =>
  request('/api/alerts/test', z.object({ delivered: z.literal(true) }), {
    method: 'POST',
    body: '{}',
    signal,
  })
export const alertOptions = (userID: number) =>
  queryOptions({
    queryKey: ['workspace-alerts', userID],
    queryFn: ({ signal }) => request('/api/alerts', alertSchema, { signal }),
    refetchInterval: 30_000,
  })
export const saveAlerts = (input: AlertInput, signal: AbortSignal) =>
  request('/api/alerts', alertSchema, {
    method: 'PUT',
    body: JSON.stringify(input),
    signal,
  })
export function alertError(error: unknown) {
  if (error instanceof ApiError) {
    if (error.code === 'demo_read_only') return 'demoReadOnly'
    if (error.code === 'invalid_alert_settings') return 'alertsInvalid'
    if (error.code === 'alert_delivery_failed') return 'alertsTestFailed'
    if (error.code === 'alert_test_cooling') return 'alertsTestCooling'
  }
  return 'alertsUnavailable'
}
export const alertNames = {
  account_reauthorization: 'alertReauthorization',
  pool_unavailable: 'alertPoolUnavailable',
  request_failures: 'alertRequestFailures',
  model_unavailable: 'alertModelUnavailable',
  quota_low: 'alertQuotaLow',
} as const
