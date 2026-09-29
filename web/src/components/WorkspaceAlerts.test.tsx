import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { WorkspaceAlerts } from './WorkspaceAlerts'
import { createQueryClient } from '@/lib/query'
import { authKey } from '@/lib/auth'
import { authenticated } from '@/test/fixtures'

it('saves optional workspace notifications and clears the entered secret', async () => {
  const client = createQueryClient()
  client.setQueryData(authKey, authenticated)
  const saved: unknown[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === 'PUT') saved.push(JSON.parse(String(init.body)))
      return Promise.resolve(
        new Response(
          JSON.stringify({
            enabled: saved.length > 0,
            configured: saved.length > 0,
            destination: saved.length ? 'hooks.example.test' : '',
            last_delivered_at: 0,
            next_retry_at: 0,
            delivery_failed: false,
            incidents: [],
          }),
        ),
      )
    }),
  )
  render(
    <QueryClientProvider client={client}>
      <WorkspaceAlerts userID={1} />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'Configure alerts' }),
  )
  await user.type(
    screen.getByLabelText('Webhook URL'),
    'https://hooks.example.test/synthetic-secret',
  )
  await user.click(screen.getByLabelText('Enable notifications'))
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(await screen.findByText('hooks.example.test')).toBeTruthy()
  expect(
    screen.queryByDisplayValue('https://hooks.example.test/synthetic-secret'),
  ).toBeNull()
  expect(saved).toEqual([
    {
      enabled: true,
      url: 'https://hooks.example.test/synthetic-secret',
      clear: false,
    },
  ])
})

it('explains that demo alerts are read-only without sending a mutation', async () => {
  const client = createQueryClient()
  client.setQueryData(authKey, {
    ...authenticated,
    demo: { username: 'demo', password: 'synthetic-demo-password' },
  })
  const fetcher = vi.fn(() =>
    Promise.resolve(
      new Response(
        JSON.stringify({
          enabled: false,
          configured: false,
          destination: '',
          last_delivered_at: 0,
          next_retry_at: 0,
          delivery_failed: false,
          incidents: [],
        }),
      ),
    ),
  )
  vi.stubGlobal('fetch', fetcher)
  render(
    <QueryClientProvider client={client}>
      <WorkspaceAlerts userID={1} />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'Configure alerts' }),
  )
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  expect((await screen.findByRole('alert')).textContent).toBe(
    'Changes are unavailable in this demo. You can still browse and filter the sample data.',
  )
  expect(fetcher).toHaveBeenCalledTimes(1)
})
