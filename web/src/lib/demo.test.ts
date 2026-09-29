import { expect, it } from 'vitest'
import { accountErrorKey } from './accounts'
import { groupErrorKey } from './groups'
import { allocationErrorKey } from './allocations'
import { proxyErrorKey } from './proxies'
import { backupErrorKey } from './backup'
import { versionErrorKey } from './version'
import { usageReasonKey } from './usage'
import { ApiError } from './request'

it.each([
  accountErrorKey,
  groupErrorKey,
  allocationErrorKey,
  proxyErrorKey,
  backupErrorKey,
  versionErrorKey,
  usageReasonKey,
])(
  'explains demo restrictions instead of suggesting a retry (%#)',
  (message) => {
    expect(message(new ApiError('demo_read_only', 403))).toBe('demoReadOnly')
  },
)
