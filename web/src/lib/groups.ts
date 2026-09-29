import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { authKey, type AuthState } from './auth'
import { ApiError, request } from './request'

const groupSchema = z.object({
  id: z.number().int().positive(),
  name: z.string().min(1),
  enabled: z.boolean(),
  is_default: z.boolean(),
  restricted_models: z.boolean(),
  created_at: z.number().int().nonnegative(),
  updated_at: z.number().int().nonnegative(),
  account_count: z.number().int().nonnegative(),
  member_count: z.number().int().nonnegative(),
})
const detailSchema = groupSchema.extend({
  account_ids: z.array(z.string()).max(100),
  allowed_models: z.array(z.string()).max(100),
})
const choicesSchema = z.object({
  groups: z
    .array(
      groupSchema.pick({ id: true, name: true }).extend({
        account_count: z.number().int().nonnegative(),
        scheme_id: z.number().int().optional(),
        scheme_name: z.string().optional(),
      }),
    )
    .max(32),
})
const grantsSchema = z.object({
  group_ids: z.array(z.number().int().positive()).max(32),
})
export type Group = z.infer<typeof groupSchema>
export type GroupDetail = z.infer<typeof detailSchema>
export const groupOptions = queryOptions({
  queryKey: ['groups'],
  queryFn: ({ signal }) =>
    request('/api/groups', z.object({ groups: z.array(groupSchema).max(32) }), {
      signal,
    }),
})
export function groupDetails(id: number, signal?: AbortSignal) {
  return request(`/api/groups/${id}`, detailSchema, { signal })
}
export function saveGroup({
  id,
  ...input
}: {
  id?: number
  name: string
  enabled: boolean
  account_ids: string[]
  model_policy: { restricted: boolean; models: string[] }
}) {
  return request(id ? `/api/groups/${id}` : '/api/groups', detailSchema, {
    method: id ? 'PATCH' : 'POST',
    body: JSON.stringify(input),
  })
}
export function memberGroupOptions(id: number) {
  return queryOptions({
    queryKey: ['member-groups', id],
    queryFn: ({ signal }) =>
      request(`/api/groups/members/${id}`, grantsSchema, { signal }),
  })
}
export function poolMembersOptions(id: number) {
  return queryOptions({
    queryKey: ['pool-members', id],
    queryFn: ({ signal }) =>
      request(
        `/api/groups/${id}/members`,
        z.object({
          members: z
            .array(
              z.object({
                id: z.number().int().positive(),
                username: z.string(),
              }),
            )
            .max(100),
        }),
        { signal },
      ),
  })
}
export function saveMemberGroups({
  id,
  group_ids,
}: {
  id: number
  group_ids: number[]
}) {
  return request(`/api/groups/members/${id}`, grantsSchema, {
    method: 'PUT',
    body: JSON.stringify({ group_ids }),
  })
}
export function availableGroupOptions(client: QueryClient, userID: number) {
  return queryOptions({
    queryKey: ['available-groups', userID],
    queryFn: ({ signal }) =>
      request('/api/keys/groups', choicesSchema, { signal }),
    enabled: () => client.getQueryData<AuthState>(authKey)?.user?.id === userID,
  })
}
export function groupErrorKey(error: Error) {
  if (error instanceof ApiError) {
    if (error.code === 'demo_read_only') return 'demoReadOnly'
    if (error.code === 'allocation_pool_locked') return 'allocationPoolConflict'
    if (error.code === 'group_exists') return 'groupNameTaken'
    if (error.code === 'group_limit') return 'groupLimitReached'
    if (error.code === 'invalid_group_input') return 'groupInputInvalid'
  }
  return 'groupSaveFailed'
}
