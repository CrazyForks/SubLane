import { z } from 'zod'
import { request } from './request'
import { gatewayStatusSchema } from './connection'
import { setupSchema } from './activation'

const systemSchema = z.object({
  name: z.literal('SubLane'),
  version: z.string(),
  status: z.literal('ok'),
  uptime_seconds: z.number().int().nonnegative(),
  storage: z.object({
    engine: z.literal('sqlite'),
    status: z.literal('ready'),
  }),
  gateway: z.object({
    provider: z.enum(['codex', 'multi']),
    status: gatewayStatusSchema,
    has_usable_key: z.boolean(),
    setup: setupSchema.optional(),
  }),
})

export async function getSystem(signal?: AbortSignal) {
  return request('/api/system', systemSchema, { signal })
}
