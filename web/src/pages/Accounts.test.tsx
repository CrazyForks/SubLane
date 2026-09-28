import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import * as queries from '@/lib/query'
import { authenticated } from '@/test/fixtures'

const account = {
  id: 'synthetic-account',
  name: 'Test subscription',
  email: 'member@example.test',
  plan: 'plus',
  enabled: true,
  status: 'unverified',
  expires_at: 0,
  created_at: 1,
  updated_at: 1,
}
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status })
function open() {
  const client = queries.createQueryClient()
  vi.spyOn(queries, 'createQueryClient').mockReturnValue(client)
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/accounts'] }),
      )}
    />,
  )
  return client
}

it('imports a subscription and clears credential input after success', async () => {
  let accounts: (typeof account)[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts/import' && init?.method === 'POST') {
        accounts = [account]
        return Promise.resolve(response(account, 201))
      }
      return Promise.resolve(response({ accounts }))
    }),
  )
  const user = userEvent.setup()
  const client = open()
  await screen.findByText('No subscription accounts')
  await user.click(screen.getByRole('button', { name: 'Add account' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(
    within(dialog).getByLabelText('Account name'),
    'Test subscription',
  )
  await user.click(
    within(dialog).getByRole('button', {
      name: 'Import auth.json',
    }),
  )
  await user.click(within(dialog).getByLabelText('auth.json contents'))
  await user.paste(
    '{"tokens":{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","account_id":"upstream-test"}}',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Import account' }),
  )
  await screen.findByText('Test subscription')
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(screen.queryByDisplayValue(/synthetic-access/)).toBeNull()
  await waitFor(() =>
    expect(client.getMutationCache().getAll()).toHaveLength(0),
  )
})

it('starts browser authorization and submits only the callback for the matching attempt', async () => {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push(url)
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts/oauth')
        return Promise.resolve(
          response(
            {
              url: 'https://auth.openai.com/oauth/authorize?state=synthetic-state',
              state: 'synthetic-state',
              expires_at: 9999999999,
            },
            201,
          ),
        )
      if (url === '/api/accounts/oauth/complete') {
        expect(JSON.parse(String(init?.body))).toEqual({
          state: 'synthetic-state',
          callback_url:
            'http://localhost:1455/auth/callback?state=synthetic-state&code=synthetic-code',
        })
        return Promise.resolve(response({ ...account, status: 'ready' }, 201))
      }
      return Promise.resolve(response({ accounts: [] }))
    }),
  )
  const user = userEvent.setup()
  open()
  await screen.findByText('No subscription accounts')
  await user.click(screen.getByRole('button', { name: 'Add account' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(
    within(dialog).getByLabelText('Account name'),
    'Test subscription',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Start authorization' }),
  )
  await within(dialog).findByRole('link', {
    name: 'Open provider authorization',
  })
  await user.type(
    within(dialog).getByLabelText('Callback URL'),
    'http://localhost:1455/auth/callback?state=synthetic-state&code=synthetic-code',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Complete authorization' }),
  )
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(calls).toContain('/api/accounts/oauth/complete')
})

it('keeps quota, identity and accessible actions together for each account', async () => {
  const calls: string[] = []
  const now = Math.floor(Date.now() / 1000)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      calls.push(url)
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url.endsWith('/check'))
        return Promise.resolve(response({ account, models: [] }))
      if (url.endsWith('/usage'))
        return Promise.resolve(
          response({
            updated_at: now,
            server_time: now,
            expires_at: now + 120,
            stale: false,
            refreshing: false,
            refresh_failed: false,
            retry_after_seconds: 0,
            limits: [
              {
                name: '',
                allowed: true,
                limit_reached: false,
                windows: [
                  {
                    kind: 'secondary',
                    used_percent: 21,
                    window_seconds: 604800,
                    reset_at: now + 3600,
                  },
                ],
              },
            ],
          }),
        )
      return Promise.resolve(response({ accounts: [account] }))
    }),
  )
  open()
  const row = await screen.findByRole('article', { name: 'Test subscription' })
  expect(within(row).getByText('member@example.test')).toBeTruthy()
  await within(row).findByText('79% remaining')
  expect(
    within(row).getByRole('button', {
      name: 'Refresh usage for Test subscription',
    }),
  ).toBeTruthy()
  await userEvent.setup().click(
    within(row).getByRole('button', {
      name: 'Verify connection for Test subscription',
    }),
  )
  await waitFor(() =>
    expect(calls).toContain('/api/accounts/synthetic-account/check'),
  )
  expect(
    within(row).getByRole('button', { name: 'Actions for Test subscription' }),
  ).toBeTruthy()
})

it('binds an account to a named network proxy', async () => {
  const proxy = {
    id: 'synthetic-proxy',
    name: 'Synthetic exit',
    endpoint: 'http://127.0.0.1:18080',
    account_count: 0,
    created_at: 1,
    updated_at: 1,
  }
  let current = { ...account, proxy_id: '' }
  const fetch = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/proxies')
        return Promise.resolve(response({ proxies: [proxy] }))
      if (
        url === '/api/accounts/synthetic-account/proxy' &&
        init?.method === 'PUT'
      ) {
        expect(JSON.parse(String(init.body))).toEqual({ proxy_id: proxy.id })
        current = { ...current, proxy_id: proxy.id }
        return Promise.resolve(response(current))
      }
      if (url === '/api/accounts')
        return Promise.resolve(response({ accounts: [current] }))
      return Promise.resolve(response({ accounts: [] }))
    })
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  open()
  const row = await screen.findByRole('article', { name: 'Test subscription' })
  await user.click(
    within(row).getByRole('button', { name: 'Actions for Test subscription' }),
  )
  await user.click(
    await screen.findByRole('menuitem', { name: 'Network proxy' }),
  )
  const dialog = await screen.findByRole('dialog')
  await user.click(
    within(dialog).getByRole('button', { name: /Network proxy/ }),
  )
  await user.type(
    screen.getByRole('searchbox', { name: 'Search proxies' }),
    'exit',
  )
  await user.click(
    screen.getByRole('menuitemradio', { name: 'Synthetic exit' }),
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Save proxy binding' }),
  )
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith(
      '/api/accounts/synthetic-account/proxy',
      expect.objectContaining({ method: 'PUT' }),
    ),
  )
  await screen.findByText('Network proxy: Synthetic exit')
})

it('searches network proxies when connecting an account and submits the selected id', async () => {
  const proxies = [
    {
      id: 'mock-east',
      name: 'Mock East',
      endpoint: 'http://east.example.test',
      account_count: 0,
      created_at: 1,
      updated_at: 1,
    },
    {
      id: 'mock-west',
      name: 'Mock West',
      endpoint: 'http://west.example.test',
      account_count: 0,
      created_at: 1,
      updated_at: 1,
    },
  ]
  let imported: Record<string, string> | undefined
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/proxies') return Promise.resolve(response({ proxies }))
      if (url === '/api/accounts/import') {
        imported = JSON.parse(String(init?.body)) as Record<string, string>
        return Promise.resolve(
          response({ ...account, proxy_id: imported.proxy_id }, 201),
        )
      }
      return Promise.resolve(response({ accounts: [] }))
    }),
  )
  const user = userEvent.setup()
  open()
  await screen.findByText('No subscription accounts')
  await user.click(screen.getByRole('button', { name: 'Add account' }))
  const dialog = await screen.findByRole('dialog')
  await user.click(
    within(dialog).getByRole('button', { name: /Network proxy/ }),
  )
  const search = screen.getByRole('searchbox', { name: 'Search proxies' })
  await user.type(search, 'missing')
  expect(screen.getByText('No matching proxies.')).toBeTruthy()
  await user.clear(search)
  await user.type(search, 'west')
  expect(screen.getByRole('menuitemradio', { name: 'Mock West' })).toBeTruthy()
  expect(screen.queryByRole('menuitemradio', { name: 'Mock East' })).toBeNull()
  await user.keyboard('{ArrowDown}{Enter}')
  expect(
    within(dialog).getByRole('button', { name: 'Network proxy: Mock West' }),
  ).toBeTruthy()
  await user.click(
    within(dialog).getByRole('button', { name: 'Network proxy: Mock West' }),
  )
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('searchbox', { name: 'Search proxies' })).toBeNull()
  expect(screen.getByRole('dialog')).toBeTruthy()
  await user.type(
    within(dialog).getByLabelText('Account name'),
    'Synthetic account',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Import auth.json' }),
  )
  await user.click(within(dialog).getByLabelText('auth.json contents'))
  await user.paste(
    '{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","account_id":"synthetic-id"}',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Import account' }),
  )
  await waitFor(() => expect(imported?.proxy_id).toBe('mock-west'))
})

it('shows three providers with only Codex selectable and starts its authorization', async () => {
  const fetch = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts/oauth') {
        expect(JSON.parse(String(init?.body)).provider).toBe('codex')
        return Promise.resolve(
          response(
            {
              url: 'https://auth.openai.com/oauth/authorize?state=synthetic-state',
              state: 'synthetic-state',
              expires_at: 9999999999,
              callback_url: 'http://localhost:1455/auth/callback',
            },
            201,
          ),
        )
      }
      return Promise.resolve(response({ accounts: [] }))
    })
  vi.stubGlobal('fetch', fetch)
  open()
  await screen.findByText('No subscription accounts')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Add account' }))
  const choices = screen.getByRole('group', { name: 'Service provider' })
  expect(within(choices).getAllByRole('button')).toHaveLength(3)
  expect(
    within(choices).getByRole('button', { name: 'Codex', pressed: true }),
  ).toBeTruthy()
  expect(
    (
      within(choices).getByRole('button', {
        name: 'Claude',
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
  expect(
    (
      within(choices).getByRole('button', {
        name: 'Antigravity (Gemini)',
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
  await user.type(screen.getByLabelText('Account name'), 'Synthetic Codex')
  await user.click(screen.getByRole('button', { name: 'Start authorization' }))
  expect(
    (
      await screen.findByRole('link', { name: 'Open provider authorization' })
    ).getAttribute('href'),
  ).toContain('auth.openai.com/oauth/authorize')
  expect(
    screen.getByPlaceholderText('http://localhost:1455/auth/callback?...'),
  ).toBeTruthy()
  for (const button of within(choices).getAllByRole('button')) {
    expect((button as HTMLButtonElement).disabled).toBe(true)
  }
})

it('shows a stored legacy account as unavailable without reconnect actions', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      return Promise.resolve(
        response({
          accounts: [{ ...account, provider: 'claude', status: 'ready' }],
        }),
      )
    }),
  )
  open()
  const card = (await screen.findByText('Test subscription')).closest(
    'article',
  )!
  expect(
    within(card).getAllByText('This provider is temporarily disabled.'),
  ).toHaveLength(2)
  expect(
    (
      within(card).getByRole('button', {
        name: 'Verify connection for Test subscription',
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
  await userEvent.setup().click(
    within(card).getByRole('button', {
      name: 'Actions for Test subscription',
    }),
  )
  expect(screen.queryByRole('menuitem', { name: 'Reauthorize' })).toBeNull()
  expect(screen.queryByRole('menuitem', { name: 'Enable account' })).toBeNull()
  expect(screen.getByRole('menuitem', { name: 'Remove account' })).toBeTruthy()
})

it.each([1, 8, 30])(
  'updates an account concurrency limit to %i from scheduling settings',
  async (nextLimit) => {
    let limit = 2
    const fetch = vi
      .fn()
      .mockImplementation((url: string, init?: RequestInit) => {
        if (url === '/api/auth/state')
          return Promise.resolve(response(authenticated))
        if (url.endsWith('/limits')) {
          limit = JSON.parse(String(init?.body)).max_concurrency
          return Promise.resolve(
            response({ ...account, max_concurrency: limit }),
          )
        }
        if (url === '/api/accounts/runtime')
          return Promise.resolve(
            response({
              accounts: [
                {
                  id: account.id,
                  max_concurrency: limit,
                  in_flight: 0,
                  cooldown_until: 0,
                  reason: '',
                  failures: 0,
                  state: 'available',
                },
              ],
              server_time: 1900000000,
            }),
          )
        return Promise.resolve(
          response({
            accounts: [{ ...account, enabled: false, max_concurrency: limit }],
          }),
        )
      })
    vi.stubGlobal('fetch', fetch)
    open()
    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('button', {
        name: 'Actions for Test subscription',
      }),
    )
    await user.click(
      screen.getByRole('menuitem', { name: 'Scheduling settings' }),
    )
    const dialog = await screen.findByRole('dialog', {
      name: 'Scheduling settings',
    })
    const input = within(dialog).getByLabelText('Concurrent model requests')
    for (const invalid of [0, 31, 1.5]) {
      await user.clear(input)
      await user.type(input, String(invalid))
      await user.click(
        within(dialog).getByRole('button', { name: 'Save settings' }),
      )
      expect(within(dialog).getByRole('alert').textContent).toBe(
        'Enter a whole number from 1 to 30.',
      )
      expect(limit).toBe(2)
    }
    await user.clear(input)
    await user.type(input, String(nextLimit))
    await user.click(
      within(dialog).getByRole('button', { name: 'Save settings' }),
    )
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(limit).toBe(nextLimit)
  },
)

it('loads accounts and scheduling status with a saved concurrency of 30', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts')
        return Promise.resolve(
          response({
            accounts: [{ ...account, enabled: false, max_concurrency: 30 }],
          }),
        )
      if (url === '/api/accounts/runtime')
        return Promise.resolve(
          response({
            accounts: [
              {
                id: account.id,
                max_concurrency: 30,
                in_flight: 0,
                cooldown_until: 0,
                reason: '',
                failures: 0,
                state: 'available',
              },
            ],
            server_time: 1900000000,
          }),
        )
      return Promise.reject(new Error('Unexpected request: ' + url))
    }),
  )
  const client = open()
  expect(await screen.findByText('Test subscription')).toBeTruthy()
  await waitFor(() =>
    expect(client.getQueryState(['account-runtime'])?.status).toBe('success'),
  )
  expect(screen.queryByText('Could not load subscription accounts.')).toBeNull()
  expect(
    screen.queryByText('Scheduling status is temporarily unavailable.'),
  ).toBeNull()
})

it('explains why a quota-exhausted account is skipped for new sessions', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts/runtime')
        return Promise.resolve(
          response({
            accounts: [
              {
                id: account.id,
                max_concurrency: 2,
                in_flight: 0,
                cooldown_until: 0,
                reason: '',
                failures: 0,
                state: 'quota_exhausted',
                quota_state: 'exhausted',
              },
            ],
            server_time: 1900000000,
          }),
        )
      if (url.endsWith('/usage'))
        return Promise.resolve(
          response({
            limits: [],
            updated_at: 1900000000,
            server_time: 1900000000,
            expires_at: 1900000120,
            stale: false,
            refreshing: false,
            refresh_failed: false,
            retry_after_seconds: 0,
          }),
        )
      return Promise.resolve(response({ accounts: [account] }))
    }),
  )
  open()
  expect(await screen.findByText('Quota exhausted')).toBeTruthy()
  expect(
    screen.getByText(/New sessions use another available account/),
  ).toBeTruthy()
})

it('identifies an unassigned account and offers pool setup', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts')
        return Promise.resolve(
          response({
            accounts: [{ ...account, group_count: 0, enabled: false }],
          }),
        )
      if (url === '/api/accounts/runtime')
        return Promise.resolve(response({ accounts: [] }))
      return Promise.reject(new Error('Unexpected request: ' + url))
    }),
  )
  open()
  await screen.findByText('Unassigned')
  expect(
    screen
      .getByRole('link', { name: 'Assign to an account pool' })
      .getAttribute('href'),
  ).toBe('/groups')
})
