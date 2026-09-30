import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { authKey, type AuthState } from './auth'
import { catalogPath, type CatalogTarget } from './catalog'
import { request } from './request'
import { reasonKeys } from './runtime'
import type { en } from '@/locales/en'

const availabilitySchema = z.object({
  model: z.string().min(1).max(128),
  state: z.enum(['available', 'unavailable', 'unknown']),
  available: z.number().int().nonnegative(),
  unknown: z.number().int().nonnegative(),
  retry_at: z.number().int().nonnegative(),
  server_time: z.number().int(),
  reasons: z
    .array(
      z.object({
        code: z.string().max(64),
        count: z.number().int().positive(),
      }),
    )
    .max(20),
  accounts: z
    .array(
      z.object({
        id: z.string().max(64),
        name: z.string().max(256),
        provider: z.string().max(32),
        reason: z.string().max(64),
        retry_at: z.number().int().nonnegative(),
        quota_state: z.string().max(32),
        quota_used_percent: z.number().min(0).max(100).optional(),
        in_flight: z.number().int().nonnegative(),
        max_concurrency: z.number().int().nonnegative(),
      }),
    )
    .max(100)
    .optional(),
})
export type AvailabilityTarget = Extract<
  CatalogTarget,
  { kind: 'group' | 'key' }
>
export const availabilityReasons: Record<string, keyof typeof en> = {
  ...reasonKeys,
  available: 'availabilityEligible',
  account_disabled: 'availabilityDisabled',
  provider_disabled: 'availabilityProviderDisabled',
  account_reauthorization_required: 'availabilityReauthorize',
  no_accounts_available: 'availabilityNoAccounts',
}
export function availabilityOptions(
  client: QueryClient,
  target: AvailabilityTarget,
  model: string,
) {
  const userID = client.getQueryData<AuthState>(authKey)?.user?.id ?? 0
  return queryOptions({
    queryKey: ['model-availability', userID, target.kind, target.id, model],
    queryFn: ({ signal }) =>
      request(
        `${catalogPath(target)}/availability?${new URLSearchParams({ model })}`,
        availabilitySchema,
        { signal },
      ),
    enabled: () => {
      const user = client.getQueryData<AuthState>(authKey)?.user
      return Boolean(
        model &&
        userID > 0 &&
        user?.id === userID &&
        (target.kind === 'key' || user.role === 'admin'),
      )
    },
    staleTime: 5000,
  })
}
