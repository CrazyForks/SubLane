import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { authKey } from '@/lib/auth'
import { createQueryClient } from '@/lib/query'
import { authenticated } from '@/test/fixtures'
import { TimeZoneSettings } from './TimeZoneSettings'

it('updates the instance zone and immediately publishes it to the signed-in view', async () => {
  const client = createQueryClient()
  client.setQueryData(authKey, { ...authenticated, time_zone: 'UTC' })
  const fetch = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ time_zone: 'Asia/Shanghai' }), {
      status: 200,
    }),
  )
  vi.stubGlobal('fetch', fetch)
  render(
    <QueryClientProvider client={client}>
      <TimeZoneSettings userID={1} />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  const input = screen.getByRole('combobox', { name: 'Instance time zone' })
  await user.clear(input)
  await user.type(input, 'Asia/Shanghai')
  await user.click(screen.getByRole('button', { name: 'Save time zone' }))
  expect(await screen.findByText('Time zone saved.')).toBeTruthy()
  expect(client.getQueryData<{ time_zone: string }>(authKey)?.time_zone).toBe(
    'Asia/Shanghai',
  )
  expect(fetch.mock.calls[0][0]).toBe('/api/settings/timezone')
})
