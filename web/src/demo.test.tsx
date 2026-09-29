import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from './App'
import { createAppRouter } from './router'
import { anonymous, authenticated, workspaces } from './test/fixtures'

const demo = { username: 'demo', password: 'sublane-demo' }

it('signs in with the advertised demo account and keeps the read-only notice visible', async () => {
  let signedIn = false
  const fetcher = vi.fn(async (path: string, init?: RequestInit) => {
    if (path === '/api/auth/login') {
      expect(JSON.parse(String(init?.body))).toEqual(demo)
      signedIn = true
    }
    if (path.startsWith('/api/auth/'))
      return new Response(
        JSON.stringify({ ...(signedIn ? authenticated : anonymous), demo }),
      )
    if (path === '/api/workspaces')
      return new Response(JSON.stringify(workspaces))
    return new Response('{}')
  })
  vi.stubGlobal('fetch', fetcher)
  const user = userEvent.setup()
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/preferences'] }),
      )}
    />,
  )
  await user.click(await screen.findByRole('button', { name: 'Explore demo' }))
  await screen.findByRole('heading', { name: 'Workspace overview' })
  expect(screen.getByText('Read-only demo')).toBeTruthy()
  await user.click(screen.getByRole('link', { name: 'Preferences' }))
  expect(screen.getByText('Read-only demo')).toBeTruthy()
  await user.click(
    within(screen.getByRole('banner')).getByRole('button', {
      name: 'Language',
    }),
  )
  await user.click(screen.getByRole('menuitemradio', { name: '简体中文' }))
  expect(await screen.findByText('只读演示')).toBeTruthy()
})

it('blocks demo mutations locally and explains the restriction', async () => {
  const fetcher = vi.fn<
    (path: string, init?: RequestInit) => Promise<Response>
  >(async (path) => {
    if (path === '/api/auth/state')
      return new Response(JSON.stringify({ ...authenticated, demo }))
    if (path === '/api/workspaces')
      return new Response(JSON.stringify(workspaces))
    if (path === '/api/groups')
      return new Response(JSON.stringify({ groups: [] }))
    if (path === '/api/accounts')
      return new Response(JSON.stringify({ accounts: [] }))
    return new Response('{}')
  })
  vi.stubGlobal('fetch', fetcher)
  const user = userEvent.setup()
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/groups'] }),
      )}
    />,
  )
  await user.click(
    await screen.findByRole('button', { name: 'Create account pool' }),
  )
  await user.type(
    screen.getByRole('textbox', { name: 'Pool name' }),
    'Synthetic pool',
  )
  await user.click(screen.getByRole('button', { name: 'Save pool' }))
  await waitFor(() =>
    expect(
      within(screen.getByRole('dialog')).getByText(
        'Changes are unavailable in this demo. You can still browse and filter the sample data.',
      ),
    ).toBeTruthy(),
  )
  expect(
    fetcher.mock.calls.some(
      ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
    ),
  ).toBe(false)
})

it('does not advertise demo credentials on normal instances', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify(anonymous))),
  )
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/login'] }),
      )}
    />,
  )
  await screen.findByRole('heading', { name: 'Sign in to SubLane' })
  expect(screen.queryByRole('button', { name: 'Explore demo' })).toBeNull()
})
