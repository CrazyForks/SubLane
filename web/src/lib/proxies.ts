import { z } from 'zod'
import { ApiError, request } from './request'

export const proxySchema = z.object({
  id: z.string().min(1),
  name: z.string(),
  endpoint: z.string(),
  account_count: z.number().int().nonnegative(),
  created_at: z.number().int().nonnegative(),
  updated_at: z.number().int().nonnegative(),
  checked_at: z.number().int().nonnegative().default(0),
  reachable: z.boolean().default(false),
  exit_ip: z.string().max(45).default(''),
  country: z.string().max(2).default(''),
  region: z.string().max(128).default(''),
  city: z.string().max(128).default(''),
  latency_ms: z.number().int().nonnegative().default(0),
  check_error: z.string().max(32).default(''),
  prune_eligible: z.boolean().default(false),
})
export type Proxy = z.infer<typeof proxySchema>

export const proxyOptions = {
  queryKey: ['proxies'],
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    request(
      '/api/proxies',
      z.object({ proxies: z.array(proxySchema).max(32) }),
      {
        signal,
      },
    ),
}

export function saveProxy(input: { id?: string; name: string; url: string }) {
  return request(
    input.id ? `/api/proxies/${encodeURIComponent(input.id)}` : '/api/proxies',
    proxySchema,
    {
      method: input.id ? 'PUT' : 'POST',
      body: JSON.stringify({ name: input.name, url: input.url }),
    },
  )
}

export function deleteProxy(id: string) {
  return request(`/api/proxies/${encodeURIComponent(id)}`, z.undefined(), {
    method: 'DELETE',
    body: '{}',
  })
}

export function importProxies(text: string) {
  return request(
    '/api/proxies/import',
    z.object({ proxies: z.array(proxySchema).max(32) }),
    {
      method: 'POST',
      body: JSON.stringify({ text }),
    },
  )
}

export function checkProxy(id: string) {
  return request(`/api/proxies/${encodeURIComponent(id)}/check`, proxySchema, {
    method: 'POST',
    body: '{}',
  })
}

export function pruneFailedProxies() {
  return request(
    '/api/proxies/prune',
    z.object({ deleted_count: z.number().int().nonnegative() }),
    {
      method: 'POST',
      body: '{}',
    },
  )
}

const errors = {
  demo_read_only: 'demoReadOnly',
  invalid_proxy_input: 'proxyInvalidInput',
  proxy_not_found: 'proxyNotFound',
  proxy_in_use: 'proxyInUse',
  proxy_exists: 'proxyExists',
  proxy_limit: 'proxyLimit',
  proxy_changed: 'proxyChanged',
} as const
export function proxyErrorKey(error: unknown) {
  const code = error instanceof ApiError ? error.code : ''
  return Object.hasOwn(errors, code)
    ? errors[code as keyof typeof errors]
    : 'proxyActionFailed'
}

export function proxyImportErrorKey(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 404 && error.code === 'not_found')
      return 'proxyImportUnavailable'
    if (error.code === 'invalid_proxy_input') return 'proxyImportInvalid'
  }
  return proxyErrorKey(error)
}
