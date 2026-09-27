import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import { authKey, replaceAuthState, type AuthState } from './auth'
import { ApiError } from './request'

export { replaceAuthState } from './auth'

export function createQueryClient() {
  const onError = (error: Error) => {
    if (error instanceof ApiError && error.code === 'unauthorized') {
      return replaceAuthState(client, {
        initialized: true,
        time_zone: 'UTC',
        user: null,
      })
    }
  }
  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({ onError }),
    mutationCache: new MutationCache({ onError }),
    defaultOptions: {
      queries: {
        staleTime: 15_000,
        retry: false,
        refetchOnWindowFocus: false,
        // Private queries are administrator-only by default, including during role-change redirects.
        enabled: (query) =>
          query.queryKey[0] === authKey[0] ||
          client.getQueryData<AuthState>(authKey)?.user?.role === 'admin',
      },
    },
  })
  return client
}
