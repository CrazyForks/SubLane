import type { en } from '@/locales/en'

type Recovery = {
  hint: keyof typeof en
  to?: '/accounts' | '/groups' | '/members' | '/admin/allocations' | '/keys'
  action?: keyof typeof en
}

export function requestRecovery(
  code: string,
  administrator: boolean,
): Recovery {
  if (
    [
      'account_busy',
      'account_queue_full',
      'account_wait_timeout',
      'account_cooling',
      'member_busy',
      'member_rate_limited',
      'rate_limited',
      'timeout',
    ].includes(code)
  )
    return { hint: 'recoveryWait' }
  if (code === 'context_limit') return { hint: 'recoveryContext' }
  if (code === 'quota_exhausted')
    return {
      hint: 'recoveryQuota',
      ...(administrator
        ? ({ to: '/accounts', action: 'viewAccounts' } as const)
        : {}),
    }
  if (
    ['model_not_allowed', 'model_not_available'].includes(code) &&
    !administrator
  )
    return { hint: 'recoveryModel', to: '/keys', action: 'clientGuideAction' }
  if (
    [
      'upstream_error',
      'upstream_unavailable',
      'upstream_forbidden',
      'upstream_rejected',
      'stream_interrupted',
    ].includes(code)
  )
    return administrator
      ? { hint: 'recoveryUpstream', to: '/accounts', action: 'viewAccounts' }
      : { hint: 'recoveryContact' }
  const target = [
    'auth_required',
    'account_refresh_failed',
    'refresh_failed',
    'account_unavailable',
    'model_catalog_unavailable',
  ].includes(code)
    ? ({ to: '/accounts', action: 'viewAccounts' } as const)
    : code.startsWith('allocation_')
      ? ({ to: '/admin/allocations', action: 'recoveryAllocations' } as const)
      : [
            'model_not_allowed',
            'model_not_available',
            'group_unavailable',
          ].includes(code)
        ? ({ to: '/groups', action: 'activationPoolAction' } as const)
        : null
  if (target)
    return administrator
      ? { hint: 'recoveryAdmin', ...target }
      : { hint: 'recoveryContact' }
  return { hint: 'recoveryUnknown' }
}
