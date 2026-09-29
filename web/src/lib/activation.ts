import { z } from 'zod'

export const setupSchema = z.object({
  stage: z.enum([
    'account',
    'verify',
    'pool',
    'access',
    'key',
    'client',
    'complete',
  ]),
  has_successful_request: z.boolean(),
})
export type SetupProgress = z.infer<typeof setupSchema>

export function setupOrder(administrator: boolean) {
  return administrator
    ? (['account', 'verify', 'pool', 'key', 'client'] as const)
    : (['access', 'key', 'client'] as const)
}

export const setupSteps = {
  account: {
    title: 'activationAccount',
    hint: 'activationAccountHint',
    action: 'viewAccounts',
    to: '/accounts',
  },
  verify: {
    title: 'activationVerify',
    hint: 'activationVerifyHint',
    action: 'viewAccounts',
    to: '/accounts',
  },
  pool: {
    title: 'activationPool',
    hint: 'activationPoolHint',
    action: 'activationPoolAction',
    to: '/groups',
  },
  access: {
    title: 'activationAccess',
    hint: 'activationAccessHint',
    action: 'manageKeys',
    to: '/keys',
  },
  key: {
    title: 'activationKey',
    hint: 'activationKeyHint',
    action: 'createKeyTitle',
    to: '/keys',
  },
  client: {
    title: 'activationClient',
    hint: 'activationClientHint',
    action: 'clientGuideAction',
    to: '/keys',
  },
  complete: {
    title: 'activationComplete',
    hint: 'activationCompleteHint',
    action: 'activationRequests',
    to: '/requests',
  },
} as const
