import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { authKey, type AuthState } from './auth'
import { request } from './request'
import { setupSchema } from './activation'

export const gatewayStatusSchema = z.enum([
  'ready',
  'needs_attention',
  'not_configured',
])
export type GatewayStatus = z.infer<typeof gatewayStatusSchema>
export function connectionOptions(client: QueryClient, userID: number) {
  return queryOptions({
    queryKey: ['connection', userID],
    queryFn: ({ signal }) =>
      request(
        '/api/connection',
        z.object({
          status: gatewayStatusSchema,
          setup: setupSchema.optional(),
        }),
        {
          signal,
        },
      ),
    enabled: () => client.getQueryData<AuthState>(authKey)?.user?.id === userID,
    refetchInterval: 30_000,
  })
}
