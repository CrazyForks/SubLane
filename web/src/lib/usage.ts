import { queryOptions } from '@tanstack/react-query'
import { z } from 'zod'
import { ApiError, request } from './request'

const windowSchema = z.object({
  kind: z.enum(['primary', 'secondary', 'model']),
  used_percent: z.number().nonnegative().nullable(),
  window_seconds: z.number().int().positive().nullable(),
  reset_at: z.number().int().nonnegative().nullable(),
})
const usageSchema = z.object({
  reset_credits: z
    .number()
    .int()
    .nonnegative()
    .nullable()
    .optional()
    .transform((value) => value ?? null),
  limits: z
    .array(
      z.object({
        name: z.string(),
        allowed: z.boolean().nullable(),
        limit_reached: z.boolean().nullable(),
        windows: z.array(windowSchema).max(2),
      }),
    )
    .max(128),
  updated_at: z.number().int().positive(),
  server_time: z.number().int().positive(),
  expires_at: z.number().int().positive(),
  stale: z.boolean(),
  refreshing: z.boolean(),
  refresh_failed: z.boolean(),
  refresh_error: z.string().optional().default(''),
  retry_after_seconds: z.number().int().nonnegative(),
})
export type UsageWindow = z.infer<typeof windowSchema>

export function usageReasonKey(error: unknown) {
  const code =
    typeof error === 'string'
      ? error
      : error instanceof ApiError
        ? error.code
        : ''
  switch (code) {
    case 'demo_read_only':
      return 'demoReadOnly'
    case 'rate_limited':
    case 'upstream_rate_limited':
      return 'usageReasonRateLimited'
    case 'reauthorization_required':
    case 'account_reauthorization_required':
      return 'usageReasonReauthorize'
    case 'credential_refresh_failed':
    case 'account_refresh_failed':
      return 'usageReasonCredentialRefresh'
    case 'provider_usage_unsupported':
      return 'usageReasonUnsupported'
    case 'invalid_upstream_response':
      return 'usageReasonInvalidResponse'
    case 'upstream_rejected':
    case 'upstream_rejected_request':
      return 'usageReasonUpstreamRejected'
    case 'refresh_timeout':
      return 'usageReasonTimeout'
    case 'upstream_unavailable':
    case 'unavailable':
      return 'usageReasonUpstreamUnavailable'
    case 'gateway_busy':
      return 'usageReasonBusy'
    default:
      return 'usageReasonUnknown'
  }
}
export function usageOptions(id: string) {
  return queryOptions({
    queryKey: ['accounts', 'usage', id],
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      request(`/api/accounts/${encodeURIComponent(id)}/usage`, usageSchema, {
        signal,
      }),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    // Poll only while the server has a bounded refresh in flight; stop on network failures.
    refetchInterval: (query) =>
      query.state.status !== 'error' && query.state.data?.refreshing
        ? 1000
        : false,
  })
}

export function refreshUsage(id: string) {
  return request(
    `/api/accounts/${encodeURIComponent(id)}/usage/refresh`,
    usageSchema,
    { method: 'POST', body: '{}' },
  )
}
