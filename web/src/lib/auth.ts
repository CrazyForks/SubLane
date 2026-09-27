import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { request } from './request'

const authSchema = z
  .object({
    initialized: z.boolean(),
    time_zone: z.string().default('UTC'),
    workspace_count: z.number().int().nonnegative().optional(),
    needs_workspace: z.boolean().optional(),
    user: z
      .object({
        id: z.number().int().positive(),
        username: z.string().min(1).max(32),
        role: z.enum(['admin', 'member']),
      })
      .nullable(),
  })
  .refine((value) => value.initialized || value.user === null)

export type AuthState = z.infer<typeof authSchema>
export type User = NonNullable<AuthState['user']>
export type Role = User['role']
export const authKey = ['auth'] as const

function commitAuthState(client: QueryClient, state: AuthState) {
  // Update identity first so old administrator observers cannot refetch as a member.
  client.setQueryData(authKey, state)
  client.removeQueries({
    predicate: (query) => query.queryKey[0] !== authKey[0],
  })
}

export async function replaceAuthState(client: QueryClient, state: AuthState) {
  await client.cancelQueries()
  commitAuthState(client, state)
}

export const authOptions = () =>
  queryOptions({
    queryKey: authKey,
    queryFn: async ({ signal, client }) => {
      const state = await request('/api/auth/state', authSchema, { signal })
      const previous = client.getQueryData<AuthState>(authKey)
      if (
        previous &&
        (previous.user?.id !== state.user?.id ||
          previous.user?.role !== state.user?.role)
      ) {
        // A session change in another tab must clear private data without cancelling this auth read.
        await client.cancelQueries({
          predicate: (query) => query.queryKey[0] !== authKey[0],
        })
        commitAuthState(client, state)
      }
      return state
    },
    staleTime: 60_000,
    refetchOnWindowFocus: true,
    refetchInterval: 60_000,
  })

export function signIn(input: { username: string; password: string }) {
  return request('/api/auth/login', authSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function registerInvitation(input: {
  token: string
  username: string
  password: string
  workspace: number
}) {
  return request('/api/auth/register', authSchema, {
    method: 'POST',
    headers: { 'X-SubLane-Workspace': String(input.workspace) },
    body: JSON.stringify({
      token: input.token,
      username: input.username,
      password: input.password,
    }),
  })
}

export function setup(input: {
  username: string
  password: string
  workspace_name: string
}) {
  return request('/api/auth/setup', authSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function signOut() {
  return request('/api/auth/logout', authSchema, { method: 'POST', body: '{}' })
}

export function changePassword(input: {
  current_password: string
  new_password: string
}) {
  return request('/api/me/password', authSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
