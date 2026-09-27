import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { WorkspaceCreate } from './WorkspaceCreate'

it('creates a workspace through the global endpoint', async () => {
  const user = userEvent.setup()
  const created = vi.fn()
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        id: 3,
        name: 'Synthetic workspace',
        status: 'active',
      }),
      { status: 201 },
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
  render(
    <QueryClientProvider client={new QueryClient()}>
      <WorkspaceCreate onCreated={created} />
    </QueryClientProvider>,
  )
  await user.click(screen.getByRole('button', { name: 'Create workspace' }))
  await user.type(
    screen.getByRole('textbox', { name: 'Workspace name' }),
    'Synthetic workspace',
  )
  await user.click(screen.getByRole('button', { name: 'Save workspace' }))
  await waitFor(() => expect(created).toHaveBeenCalledWith(3))
  expect(fetchMock.mock.calls[0][0]).toBe('/api/workspaces')
})

it('explains the owner workspace limit', async () => {
  const user = userEvent.setup()
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: 'workspace_limit_reached' }), {
        status: 409,
      }),
    ),
  )
  render(
    <QueryClientProvider client={new QueryClient()}>
      <WorkspaceCreate />
    </QueryClientProvider>,
  )
  await user.click(screen.getByRole('button', { name: 'Create workspace' }))
  await user.type(
    screen.getByRole('textbox', { name: 'Workspace name' }),
    'Synthetic workspace',
  )
  await user.click(screen.getByRole('button', { name: 'Save workspace' }))
  expect(
    await screen.findByText('You can own up to 10 workspaces.'),
  ).toBeTruthy()
})
