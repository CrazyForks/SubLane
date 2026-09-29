import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { DeviceAuthorization } from './DeviceAuthorization'

function mount(expires_at: number, onRestart = vi.fn()) {
  const onConnected = vi.fn().mockResolvedValue(undefined)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <DeviceAuthorization
        authorization={{
          state: 'synthetic-state',
          user_code: 'MOCK-CODE',
          interval: 5,
          expires_at,
        }}
        onConnected={onConnected}
        onRestart={onRestart}
      />
    </QueryClientProvider>,
  )
  return { client, onConnected, onRestart }
}

it('stops polling after access denial and offers restart', async () => {
  const fetcher = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ error: 'oauth_access_denied' }), {
      status: 400,
    }),
  )
  vi.stubGlobal('fetch', fetcher)
  const { onConnected, onRestart, client } = mount(
    Math.floor(Date.now() / 1000) + 600,
  )
  await screen.findByRole('alert')
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Start again' }))
  expect(onRestart).toHaveBeenCalledOnce()
  expect(onConnected).not.toHaveBeenCalled()
  await waitFor(() => expect(fetcher).toHaveBeenCalledOnce())
  client.clear()
})

it('does not poll an expired device attempt', async () => {
  const fetcher = vi.fn()
  vi.stubGlobal('fetch', fetcher)
  const { onConnected, client } = mount(1)
  expect(screen.getByRole('alert').textContent).toContain('expired')
  expect(fetcher).not.toHaveBeenCalled()
  expect(onConnected).not.toHaveBeenCalled()
  client.clear()
})
