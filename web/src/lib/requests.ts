import { z } from 'zod'
import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { authKey, type AuthState } from './auth'
import { request } from './request'
import { providers } from './accounts'

export function cacheHitRate(input: number | null, cached: number | null) {
  // Missing or inconsistent usage must not look like a cache miss or a 100% hit.
  if (
    input === null ||
    cached === null ||
    input <= 0 ||
    cached < 0 ||
    cached > input
  )
    return null
  return cached / input
}

export const outcomes = [
  'success',
  'incomplete',
  'error',
  'canceled',
  'rejected',
] as const
export const outcomeKeys = {
  success: 'requestSuccess',
  incomplete: 'requestIncomplete',
  error: 'requestFailed',
  canceled: 'requestCanceled',
  rejected: 'requestRejected',
} as const
const tokens = z.number().int().nonnegative().nullable()
const recordSchema = z.object({
  id: z.number().int().positive(),
  user_id: z.number().int(),
  key_id: z.number().int(),
  group_id: z.number().int(),
  account_id: z.string(),
  provider: z.union([z.enum(providers), z.literal('')]),
  model: z.string(),
  reasoning_effort: z.string().max(32).default(''),
  transport: z.enum(['http', 'websocket']),
  operation: z.enum(['responses', 'chat', 'compact', 'messages', 'gemini']),
  started_at: z.number().int(),
  duration_ms: z.number().int().nonnegative(),
  request_id: z.string().max(64),
  first_token_ms: tokens,
  outcome: z.enum(outcomes),
  error_code: z.string(),
  upstream_status: z.number().int().nullable(),
  input_tokens: tokens,
  output_tokens: tokens,
  cached_tokens: tokens,
  username: z.string(),
  key_name: z.string(),
  group_name: z.string(),
  account_name: z.string(),
})
export type RequestRecord = z.infer<typeof recordSchema>
export type RequestFilters = {
  account_id?: string
  outcome?: string
  model?: string
  request_id?: string
  from?: number
  until?: number
  key_id?: number
  user_id?: number
}

function currentScope(
  client: QueryClient,
  userID: number | undefined,
  scope: 'personal' | 'all',
) {
  const user = client.getQueryData<AuthState>(authKey)?.user
  return (
    userID !== undefined &&
    user?.id === userID &&
    (scope === 'personal' || user.role === 'admin')
  )
}

export function requestOptions(
  client: QueryClient,
  userID: number | undefined,
  scope: 'personal' | 'all',
  cursor: number,
  filters: RequestFilters,
) {
  const query = new URLSearchParams({ cursor: String(cursor) })
  for (const [key, value] of Object.entries(filters)) {
    if (scope === 'personal' && (key === 'account_id' || key === 'user_id'))
      continue
    if (value !== undefined && value !== '') query.set(key, String(value))
  }
  return queryOptions({
    queryKey: ['request-history', userID, scope, cursor, filters],
    enabled: () => currentScope(client, userID, scope),
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      request(
        `${scope === 'personal' ? '/api/me/requests' : '/api/requests'}?${query}`,
        z.object({
          requests: z.array(recordSchema).max(50),
          next_cursor: z.number().int().nonnegative(),
        }),
        { signal },
      ),
  })
}

export function requestCallerOptions(
  client: QueryClient,
  userID: number,
  scope: 'personal' | 'all',
) {
  return queryOptions({
    queryKey: ['request-callers', userID, scope],
    enabled: () => currentScope(client, userID, scope),
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      request(
        `${scope === 'personal' ? '/api/me/requests' : '/api/requests'}/filters`,
        z.object({
          callers: z
            .array(
              z.object({
                user_id: z.number().int(),
                key_id: z.number().int(),
                username: z.string(),
                key_name: z.string(),
              }),
            )
            .max(5000),
        }),
        { signal },
      ),
    staleTime: 30000,
  })
}
