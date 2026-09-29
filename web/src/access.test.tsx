import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from './App'
import { createAppRouter } from './router'
import {
  authenticated,
  memberAuthenticated,
  system,
  workspaces,
} from './test/fixtures'

vi.mock('./pages/Usage', () => ({ Usage: () => null, TeamUsage: () => null }))

function open(path: string) {
  render(
    <App
      router={createAppRouter(createMemoryHistory({ initialEntries: [path] }))}
    />,
  )
}

function memberSession() {
  const fetchMock = vi.fn().mockImplementation((url: string) => {
    if (url === '/api/workspaces')
      return Promise.resolve(new Response(JSON.stringify(workspaces)))
    if (url === '/api/connection')
      return Promise.resolve(
        new Response(
          JSON.stringify({
            status: 'not_configured',
            setup: { stage: 'access', has_successful_request: false },
          }),
        ),
      )
    if (url !== '/api/auth/state')
      return Promise.reject(new Error('Unexpected management request'))
    return Promise.resolve(new Response(JSON.stringify(memberAuthenticated)))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

it('gives members their own workspace without management navigation or requests', async () => {
  const fetchMock = memberSession()
  const user = userEvent.setup()
  open('/')
  await screen.findByRole('heading', { name: 'Your workspace' })
  await screen.findByRole('heading', { name: 'Complete your first request' })
  expect(screen.getAllByRole('link', { name: 'Manage API keys' })).toHaveLength(
    1,
  )
  expect(screen.queryByRole('link', { name: 'Accounts' })).toBeNull()
  expect(screen.queryByRole('link', { name: 'Members' })).toBeNull()
  expect(screen.getByRole('link', { name: 'Requests' })).toBeTruthy()
  expect(screen.queryByRole('link', { name: 'All requests' })).toBeNull()
  expect(screen.queryByRole('link', { name: 'Resource allowances' })).toBeNull()
  expect(screen.queryByRole('group', { name: 'Administration' })).toBeNull()
  await user.click(screen.getByRole('link', { name: 'Preferences' }))
  await screen.findByRole('heading', { name: 'Preferences' })
  expect(
    fetchMock.mock.calls.every(
      ([url]) =>
        url === '/api/auth/state' ||
        url === '/api/workspaces' ||
        url === '/api/connection',
    ),
  ).toBe(true)
})

it('groups common and administrator navigation separately for administrators', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation((url: string) =>
        Promise.resolve(
          new Response(
            JSON.stringify(url === '/api/auth/state' ? authenticated : system),
          ),
        ),
      ),
  )
  open('/')
  await screen.findByRole('heading', { name: 'Workspace overview' })
  const general = screen.getByRole('group', { name: 'General' })
  const administration = screen.getByRole('group', { name: 'Administration' })
  expect(within(general).getByRole('link', { name: 'Overview' })).toBeTruthy()
  expect(within(general).getByRole('link', { name: 'Requests' })).toBeTruthy()
  expect(
    within(administration).getByRole('link', { name: 'All requests' }),
  ).toBeTruthy()
  expect(
    within(administration).getByRole('link', { name: 'Accounts' }),
  ).toBeTruthy()
  expect(
    within(administration).getByRole('link', { name: 'Members' }),
  ).toBeTruthy()
  expect(
    within(administration).getByRole('link', { name: 'Resource allowances' }),
  ).toBeTruthy()
})

it.each([
  '/accounts',
  '/members',
  '/groups',
  '/admin/requests',
  '/admin/usage',
  '/admin/allocations',
  '/admin/settings',
  '/admin/settings/codex',
  '/admin/settings/backup',
  '/admin/settings/alerts',
])(
  'denies member direct access to %s before loading management data',
  async (path) => {
    const fetchMock = memberSession()
    const user = userEvent.setup()
    open(path)
    await screen.findByRole('heading', { name: 'Access denied' })
    await user.click(screen.getByRole('link', { name: 'Back to overview' }))
    await screen.findByRole('heading', { name: 'Your workspace' })
    expect(
      fetchMock.mock.calls.every(
        ([url]) =>
          url === '/api/auth/state' ||
          url === '/api/workspaces' ||
          url === '/api/connection',
      ),
    ).toBe(true)
  },
)

it.each([undefined, 'unknown'])(
  'does not accept a session with role %s',
  async (role) => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            ...memberAuthenticated,
            user: { ...memberAuthenticated.user, role },
          }),
        ),
      ),
    )
    open('/accounts')
    await screen.findByRole('heading', { name: 'Unable to connect' })
    expect(screen.queryByRole('link', { name: 'Accounts' })).toBeNull()
  },
)
