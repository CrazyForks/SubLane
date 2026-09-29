import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import type { AuthState } from '@/lib/auth'
import { i18n } from '@/lib/i18n'
import { authenticated, memberAuthenticated } from '@/test/fixtures'

const price = {
  model: 'synthetic-alpha',
  input: 1250000,
  cached: 125000,
  output: 10000000,
  source: 'remote',
}
const catalog = {
  unit: 'micro_usd_per_million_tokens',
  prices: [price, { ...price, model: 'synthetic-beta', source: 'override' }],
}
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status })

function mount(
  data: unknown = catalog,
  identity: AuthState = authenticated,
  status = 200,
) {
  const fetch = vi.fn().mockImplementation((url: string) => {
    if (url === '/api/auth/state') return Promise.resolve(response(identity))
    if (url === '/api/pricing') return Promise.resolve(response(data, status))
    return Promise.reject(new Error('Unexpected request'))
  })
  vi.stubGlobal('fetch', fetch)
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/pricing'] }),
      )}
    />,
  )
  return fetch
}

it.each([authenticated, memberAuthenticated])(
  'exposes model prices to $user.role through shared navigation with correct units',
  async (identity) => {
    mount(catalog, identity)
    const heading = await screen.findByRole('heading', { name: 'Model prices' })
    expect(heading).toBeTruthy()
    expect(
      screen.getByRole('link', { name: 'Model prices' }).getAttribute('href'),
    ).toBe('/pricing')
    const row = (await screen.findByText('synthetic-alpha')).closest('tr')!
    expect(within(row).getByText('$1.25')).toBeTruthy()
    expect(within(row).getByText('$0.125')).toBeTruthy()
    expect(within(row).getByText('$10')).toBeTruthy()
    expect(within(row).getByText('Remote catalog')).toBeTruthy()
    expect(screen.getByText('USD per 1 million tokens')).toBeTruthy()
    const user = userEvent.setup()
    await user.type(
      screen.getByRole('textbox', { name: 'Search model prices' }),
      ' BETA',
    )
    expect(screen.getByText('synthetic-beta')).toBeTruthy()
    expect(screen.queryByText('synthetic-alpha')).toBeNull()
  },
)

it('bounds the rendered catalog and resets pagination when searching', async () => {
  mount({
    ...catalog,
    prices: Array.from({ length: 51 }, (_, index) => ({
      ...price,
      model: `synthetic-${String(index).padStart(3, '0')}`,
    })),
  })
  await screen.findByText('synthetic-000')
  expect(screen.queryByText('synthetic-050')).toBeNull()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Next' }))
  expect(screen.getByText('synthetic-050')).toBeTruthy()
  expect(screen.queryByText('synthetic-000')).toBeNull()
  await user.type(
    screen.getByRole('textbox', { name: 'Search model prices' }),
    '000',
  )
  expect(screen.getByText('synthetic-000')).toBeTruthy()
})

it('keeps small and free cached input prices readable', async () => {
  mount({ ...catalog, prices: [{ ...price, input: 1, cached: 0 }] })
  const row = (await screen.findByText('synthetic-alpha')).closest('tr')!
  expect(within(row).getByText('$0.000001')).toBeTruthy()
  expect(within(row).getByText('$0')).toBeTruthy()
})

it('uses compact USD amounts in Chinese without losing small-price precision', async () => {
  await i18n.changeLanguage('zh')
  mount({ ...catalog, prices: [{ ...price, input: 1 }] })
  const row = (await screen.findByText('synthetic-alpha')).closest('tr')!
  expect(within(row).getByText('$0.000001')).toBeTruthy()
  expect(within(row).getByText('$0.125')).toBeTruthy()
  expect(screen.getByText('美元 / 每 100 万 Token')).toBeTruthy()
})

it('distinguishes an empty catalog from a search without matches', async () => {
  mount({ ...catalog, prices: [] })
  expect(
    await screen.findByText(
      'No model prices are available yet. Reload after the price catalog has synced.',
    ),
  ).toBeTruthy()
  expect(screen.queryByText('No matching model prices.')).toBeNull()
})

it('rejects invalid price units and retries a failed read', async () => {
  const fetch = mount({ ...catalog, unit: 'usd_per_token' })
  expect(await screen.findByRole('alert')).toBeTruthy()
  expect(screen.queryByText('synthetic-alpha')).toBeNull()
  fetch.mockImplementation((url: string) =>
    Promise.resolve(response(url === '/api/pricing' ? catalog : authenticated)),
  )
  await userEvent.setup().click(screen.getByRole('button', { name: 'Refresh' }))
  await screen.findByText('synthetic-alpha')
})

it('retains the last prices and labels them when a reload fails', async () => {
  const fetch = mount()
  await screen.findByText('synthetic-alpha')
  fetch.mockImplementation((url: string) =>
    Promise.resolve(
      url === '/api/pricing'
        ? response({ error: 'unavailable' }, 503)
        : response(authenticated),
    ),
  )
  await userEvent.setup().click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() => expect(screen.getByRole('alert')).toBeTruthy())
  expect(screen.getByText('synthetic-alpha')).toBeTruthy()
  expect(
    screen.getByText(
      'Could not reload prices. Showing the last loaded catalog.',
    ),
  ).toBeTruthy()
})
