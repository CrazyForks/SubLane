import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { authenticated } from '@/test/fixtures'

function session() {
  const fetch = vi.fn().mockImplementation((url: string) =>
    Promise.resolve(
      new Response(
        JSON.stringify(
          url === '/api/auth/state'
            ? authenticated
            : {
                manual_version: '',
                auto_sync: true,
                effective_version: '0.100.0',
                default_version: '0.100.0',
                latest_version: '',
                source: 'builtin',
                checked_at: 0,
                successful_at: 0,
                next_check_at: 0,
                server_time: 1900000000,
                retry_after_seconds: 0,
                syncing: false,
                sync_failed: false,
              },
        ),
      ),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  return fetch
}

function open(path: string) {
  const router = createAppRouter(
    createMemoryHistory({ initialEntries: [path] }),
  )
  render(<App router={router} />)
  return router
}

it('opens Codex settings from a keyboard-operable nested administrator menu', async () => {
  const fetch = session()
  const user = userEvent.setup()
  open('/preferences')
  await screen.findByRole('heading', { name: 'Preferences' })
  const parent = within(
    screen.getByRole('group', { name: 'Administration' }),
  ).getByRole('button', { name: 'System settings' })
  expect(parent.getAttribute('aria-expanded')).toBe('false')
  parent.focus()
  await user.keyboard('{Enter}')
  expect(parent.getAttribute('aria-expanded')).toBe('true')
  await user.click(screen.getByRole('link', { name: 'Codex client version' }))
  expect(
    await screen.findByRole('heading', { name: 'System settings' }),
  ).toBeTruthy()
  expect(await screen.findByLabelText('Manual version')).toBeTruthy()
  expect(fetch.mock.calls.some(([url]) => url === '/api/settings/codex')).toBe(
    true,
  )
  expect(
    screen
      .getByRole('link', { name: 'Codex client version' })
      .getAttribute('aria-current'),
  ).toBe('page')
  parent.focus()
  await user.keyboard(' ')
  expect(parent.getAttribute('aria-expanded')).toBe('false')
  expect(
    screen.queryByRole('link', { name: 'Codex client version' }),
  ).toBeNull()
  expect(screen.getByLabelText('Manual version')).toBeTruthy()
  await user.click(screen.getByRole('link', { name: 'Preferences' }))
  expect(
    await screen.findByRole('heading', { name: 'Appearance' }),
  ).toBeTruthy()
})

it.each(['/admin/settings', '/admin/settings/codex'])(
  'opens the active Codex submenu for direct navigation to %s',
  async (path) => {
    session()
    const router = open(path)
    expect(await screen.findByLabelText('Manual version')).toBeTruthy()
    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/admin/settings/codex'),
    )
    expect(
      screen
        .getByRole('button', { name: 'System settings' })
        .getAttribute('aria-expanded'),
    ).toBe('true')
    expect(
      screen
        .getByRole('link', { name: 'Codex client version' })
        .getAttribute('aria-current'),
    ).toBe('page')
  },
)

it('opens the instance time zone page from the system submenu', async () => {
  session()
  open('/admin/settings/timezone')
  expect(
    await screen.findByRole('combobox', { name: 'Instance time zone' }),
  ).toHaveProperty('value', 'UTC')
  expect(
    screen
      .getByRole('link', { name: 'Time zone' })
      .getAttribute('aria-current'),
  ).toBe('page')
})
