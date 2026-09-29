import { render, screen } from '@testing-library/react'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { authenticated, system, workspaces } from '@/test/fixtures'

it('directs an administrator with a verified unassigned account to pools', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      Promise.resolve(
        new Response(
          JSON.stringify(
            url === '/api/auth/state'
              ? authenticated
              : url === '/api/workspaces'
                ? workspaces
                : {
                    ...system,
                    gateway: {
                      ...system.gateway,
                      setup: { stage: 'pool', has_successful_request: false },
                    },
                  },
          ),
        ),
      ),
    ),
  )
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/admin/instance'] }),
      )}
    />,
  )
  expect(
    (
      await screen.findByRole('link', { name: 'Set up an account pool' })
    ).getAttribute('href'),
  ).toBe('/groups')
  expect(screen.getByText('Complete your first request')).toBeTruthy()
})
