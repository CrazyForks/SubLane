import { act, render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { createQueryClient } from '@/lib/query'
import { authKey } from '@/lib/auth'
import { memberAuthenticated } from '@/test/fixtures'
import * as ccswitch from '@/lib/ccswitch'
import { CCSwitchImport } from './CCSwitchImport'
const secret = 'sl_' + 'x'.repeat(43)
const key = {
  id: 1,
  name: 'Synthetic laptop',
  prefix: 'sl_xxxxxxxx',
  group_id: 2,
  group_name: 'Synthetic group',
  group_access: 'allowed' as const,
  enabled: true,
  copyable: true,
  created_at: 1900000000,
  last_used_at: null,
  revoked_at: null,
  expires_at: null,
}
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status })
function open(value = key) {
  const client = createQueryClient()
  client.setQueryData(authKey, memberAuthenticated)
  client.setQueryData(['model-catalog', 2, 'key', 1], {
    models: ['synthetic-model'],
    updated_at: 0,
    server_time: 1900000000,
    known: true,
    usable: true,
    stale: false,
    refreshing: false,
    refresh_failed: false,
    retry_after_seconds: 0,
    partial: false,
  })
  const result = render(
    <QueryClientProvider client={client}>
      <CCSwitchImport value={value} userID={2} usable />
    </QueryClientProvider>,
  )
  return { client, ...result }
}
it.each([
  ['OpenCode', 'opencode', '/v1'],
  ['Claude Code', 'claude', ''],
  ['OpenClaw', 'openclaw', '/v1'],
  ['Hermes', 'hermes', '/v1'],
  ['Gemini CLI', 'gemini', ''],
  ['Grok Build', 'grokbuild', '/v1'],
])(
  'prepares and launches %s from the selected client',
  async (label, app, path) => {
    const fetch = vi
      .fn()
      .mockImplementation(() => Promise.resolve(response({ secret })))
    vi.stubGlobal('fetch', fetch)
    const launch = vi
      .spyOn(ccswitch, 'openCCSwitch')
      .mockImplementation(() => {})
    const user = userEvent.setup()
    const { client } = open()
    await user.click(
      screen.getByRole('button', {
        name: 'Import Synthetic laptop into CC Switch',
      }),
    )
    const selector = screen.getByRole('button', { name: 'Client' })
    expect(selector.textContent).toContain('Codex')
    await user.click(selector)
    await user.click(screen.getByRole('menuitemradio', { name: label }))
    expect(selector.textContent).toContain(label)
    expect(screen.getByText(window.location.origin + path)).toBeTruthy()
    expect(fetch).not.toHaveBeenCalled()
    await user.type(screen.getByLabelText('Model ID'), 'synthetic-model')
    await user.click(screen.getByRole('button', { name: 'Prepare import' }))
    const ready = await screen.findByRole('button', { name: 'Open CC Switch' })
    expect(selector.hasAttribute('disabled')).toBe(true)
    expect(launch).not.toHaveBeenCalled()
    expect(document.documentElement.innerHTML).not.toContain(secret)
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((q) => q.state.data),
      ),
    ).not.toContain(secret)
    expect(
      JSON.stringify(
        client
          .getMutationCache()
          .getAll()
          .map((m) => m.state),
      ),
    ).not.toContain(secret)
    await user.click(ready)
    const link = new URL(launch.mock.calls[0][0])
    expect(link.searchParams.get('app')).toBe(app)
    expect(link.searchParams.get('endpoint')).toBe(
      window.location.origin + path,
    )
    expect(link.searchParams.get('model')).toBe('synthetic-model')
    expect(link.searchParams.get('enabled')).toBe('false')
    await user.click(screen.getByRole('button', { name: 'Edit configuration' }))
    expect(screen.queryByRole('button', { name: 'Open CC Switch' })).toBeNull()
    await user.click(selector)
    await user.click(screen.getByRole('menuitemradio', { name: 'Codex' }))
    await user.click(screen.getByRole('button', { name: 'Prepare import' }))
    await user.click(
      await screen.findByRole('button', { name: 'Open CC Switch' }),
    )
    const edited = new URL(launch.mock.calls[1][0])
    expect(edited.searchParams.get('app')).toBe('codex')
    expect(edited.searchParams.get('endpoint')).toBe(
      window.location.origin + '/v1',
    )
    expect(edited.searchParams.has('haikuModel')).toBe(false)
    expect(fetch).toHaveBeenCalledTimes(2)
  },
)
it('prepares only after a valid submission and opens the local app from a second gesture', async () => {
  const fetch = vi.fn().mockResolvedValue(response({ secret }))
  vi.stubGlobal('fetch', fetch)
  const launch = vi.spyOn(ccswitch, 'openCCSwitch').mockImplementation(() => {})
  const user = userEvent.setup()
  const { client } = open()
  await user.click(
    screen.getByRole('button', {
      name: 'Import Synthetic laptop into CC Switch',
    }),
  )
  const dialog = await screen.findByRole('dialog', {
    name: 'Import into CC Switch',
  })
  expect(fetch).not.toHaveBeenCalled()
  await user.click(
    within(dialog).getByRole('button', { name: 'Prepare import' }),
  )
  expect(fetch).not.toHaveBeenCalled()
  expect(within(dialog).getByRole('alert')).toBeTruthy()
  await user.type(within(dialog).getByLabelText('Model ID'), 'synthetic-model')
  await user.click(
    within(dialog).getByRole('button', { name: 'Prepare import' }),
  )
  const ready = await within(dialog).findByRole('button', {
    name: 'Open CC Switch',
  })
  expect(launch).not.toHaveBeenCalled()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(document.documentElement.innerHTML).not.toContain(secret)
  expect(document.querySelector('a[href^="ccswitch:"]')).toBeNull()
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((q) => q.state.data),
    ),
  ).not.toContain(secret)
  expect(
    JSON.stringify(
      client
        .getMutationCache()
        .getAll()
        .map((m) => m.state),
    ),
  ).not.toContain(secret)
  await user.click(ready)
  expect(launch).toHaveBeenCalledTimes(1)
  const link = new URL(launch.mock.calls[0][0])
  expect(link.searchParams.get('apiKey')).toBe(secret)
  expect(link.searchParams.get('model')).toBe('synthetic-model')
  expect(link.searchParams.get('enabled')).toBe('false')
  expect(
    within(dialog).getByText(
      'Confirm the import in CC Switch. If it did not open, check that the app is installed and try again.',
    ),
  ).toBeTruthy()
  await user.click(within(dialog).getByRole('button', { name: 'Done' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(document.activeElement).toBe(
    screen.getByRole('button', {
      name: 'Import Synthetic laptop into CC Switch',
    }),
  )
})
it.each([200, 401])(
  'ignores a delayed disclosure (%i) after the identity changes',
  async (status) => {
    let resolve!: (value: Response) => void
    let signal: AbortSignal | null | undefined
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
        signal = init?.signal
        return new Promise<Response>((done) => {
          resolve = done
        })
      }),
    )
    const launch = vi
      .spyOn(ccswitch, 'openCCSwitch')
      .mockImplementation(() => {})
    const user = userEvent.setup()
    const { client } = open()
    await user.click(
      screen.getByRole('button', {
        name: 'Import Synthetic laptop into CC Switch',
      }),
    )
    await user.type(screen.getByLabelText('Model ID'), 'synthetic-model')
    await user.click(screen.getByRole('button', { name: 'Prepare import' }))
    await act(async () => {
      client.setQueryData(authKey, {
        ...memberAuthenticated,
        user: { ...memberAuthenticated.user, id: 3 },
      })
      resolve(
        response(
          status === 200 ? { secret } : { error: 'unauthorized' },
          status,
        ),
      )
    })
    await waitFor(() =>
      expect(
        screen
          .getByRole('button', { name: 'Prepare import' })
          .hasAttribute('disabled'),
      ).toBe(false),
    )
    expect(launch).not.toHaveBeenCalled()
    expect(screen.queryByRole('button', { name: 'Open CC Switch' })).toBeNull()
    expect(
      client.getQueryData<typeof memberAuthenticated>(authKey)?.user?.id,
    ).toBe(3)
    expect(signal).toBeTruthy()
  },
)
it('aborts disclosure when closed and does not retain a prepared link on reopening', async () => {
  let resolve!: (value: Response) => void
  let signal: AbortSignal | null | undefined
  const fetch = vi
    .fn()
    .mockImplementation((_url: string, init?: RequestInit) => {
      signal = init?.signal
      return new Promise<Response>((done) => {
        resolve = done
      })
    })
  vi.stubGlobal('fetch', fetch)
  const launch = vi.spyOn(ccswitch, 'openCCSwitch').mockImplementation(() => {})
  const user = userEvent.setup()
  open()
  const trigger = screen.getByRole('button', {
    name: 'Import Synthetic laptop into CC Switch',
  })
  await user.click(trigger)
  await user.type(screen.getByLabelText('Model ID'), 'synthetic-model')
  await user.click(screen.getByRole('button', { name: 'Prepare import' }))
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(signal?.aborted).toBe(true))
  await act(async () => {
    resolve(response({ secret }))
  })
  await user.click(trigger)
  expect(screen.queryByRole('button', { name: 'Open CC Switch' })).toBeNull()
  expect(launch).not.toHaveBeenCalled()
})
it('blocks legacy keys and clears prepared data if a key becomes unavailable', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ secret })))
  const user = userEvent.setup()
  const legacy = open({ ...key, copyable: false })
  expect(
    screen
      .getByRole('button', { name: 'Import Synthetic laptop into CC Switch' })
      .hasAttribute('disabled'),
  ).toBe(true)
  legacy.unmount()
  const view = open()
  await user.click(
    screen.getByRole('button', {
      name: 'Import Synthetic laptop into CC Switch',
    }),
  )
  await user.type(screen.getByLabelText('Model ID'), 'synthetic-model')
  await user.click(screen.getByRole('button', { name: 'Prepare import' }))
  await screen.findByRole('button', { name: 'Open CC Switch' })
  view.rerender(
    <QueryClientProvider client={view.client}>
      <CCSwitchImport
        value={{ ...key, enabled: false }}
        userID={2}
        usable={false}
      />
    </QueryClientProvider>,
  )
  await waitFor(() =>
    expect(screen.queryByRole('button', { name: 'Open CC Switch' })).toBeNull(),
  )
})
