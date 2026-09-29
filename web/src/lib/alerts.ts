import { z } from 'zod'
import { queryOptions } from '@tanstack/react-query'
import { request } from './request'

const alertSchema = z.object({
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
        ]),
        subject: z.string().max(64),
        since: z.number().int(),
      }),
    )
    .max(1000),
})
export type AlertState = z.infer<typeof alertSchema>
export type AlertInput = { enabled: boolean; url: string; clear: boolean }
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
export const alertNames = {
  account_reauthorization: 'alertReauthorization',
  pool_unavailable: 'alertPoolUnavailable',
  request_failures: 'alertRequestFailures',
} as const
