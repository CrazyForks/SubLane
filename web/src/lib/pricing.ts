import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { authKey, type AuthState } from './auth'
import { request } from './request'

const rate = z.number().int().nonnegative().max(1_000_000_000_000)
const catalogSchema = z.object({
  unit: z.literal('micro_usd_per_million_tokens'),
  prices: z.array(
    z.object({
      model: z.string().min(1),
      input: rate,
      cached: rate,
      output: rate,
      source: z.string(),
    }),
  ),
})

export function pricingOptions(client: QueryClient) {
  const userID = client.getQueryData<AuthState>(authKey)?.user?.id ?? 0
  return queryOptions({
    queryKey: ['pricing', userID],
    queryFn: ({ signal }) => request('/api/pricing', catalogSchema, { signal }),
    enabled: () =>
      userID > 0 &&
      client.getQueryData<AuthState>(authKey)?.user?.id === userID,
  })
}
