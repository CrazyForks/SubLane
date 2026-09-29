import type { Role } from './auth'

const administratorPaths = ['/accounts', '/members', '/groups', '/admin']

export function canAccess(
  pathname: string,
  role: Role | undefined,
  platformAdmin = false,
) {
  let path: string
  try {
    path = decodeURIComponent(pathname).toLowerCase().replace(/\/+$/, '')
  } catch {
    return false
  }
  if (role === 'admin') {
    // This settings page owns workspace configuration, so platform-owner access is not required.
    if (path === '/admin/settings/alerts') return true
    if (path === '/admin/settings' || path.startsWith('/admin/settings/'))
      return platformAdmin
    return true
  }
  if (role !== 'member') return false
  return !administratorPaths.some(
    (prefix) => path === prefix || path.startsWith(prefix + '/'),
  )
}
