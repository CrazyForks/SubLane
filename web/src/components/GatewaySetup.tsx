import { Link } from '@tanstack/react-router'
import { ArrowUpRight, CircleCheck, Monitor, Terminal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from './ui/Button'
import { Status } from './Status'
import type { GatewayStatus } from '@/lib/connection'
import type { SetupProgress } from '@/lib/activation'
import { Activation } from './Activation'

export function GatewaySetup({
  status,
  hasUsableKey,
  setup,
}: {
  status: GatewayStatus | 'unknown'
  hasUsableKey: boolean | null
  setup?: SetupProgress
}) {
  const { t } = useTranslation()
  if (setup && status !== 'unknown')
    return <Activation setup={setup} administrator />
  const configured = status === 'ready'
  const ready = configured && hasUsableKey === true
  const needsKey = configured && hasUsableKey === false
  const statusKey =
    status === 'unknown'
      ? 'gatewayStatusUnknown'
      : status === 'needs_attention'
        ? 'gatewayNeedsAttention'
        : ready
          ? 'ready'
          : needsKey
            ? 'gatewayKeyNeeded'
            : 'notConfigured'
  const steps = [
    {
      title: 'administrator',
      description: 'adminReadyDescription',
      status: 'ready',
    },
    {
      title: 'codexTitle',
      description: configured
        ? 'accountReadyDescription'
        : 'codexSetupDescription',
      status: configured
        ? 'ready'
        : status === 'needs_attention'
          ? 'gatewayNeedsAttention'
          : 'needsConnection',
    },
    {
      title: 'teamAccess',
      description: hasUsableKey
        ? 'teamAccessDescription'
        : 'gatewayKeySetupDescription',
      status:
        hasUsableKey === null
          ? 'statusUnknown'
          : hasUsableKey
            ? 'ready'
            : 'gatewayNoKey',
    },
  ] as const
  return (
    <div className="space-y-6">
      <section
        aria-labelledby="gateway-title"
        className="rounded-xl border border-border bg-card"
      >
        <div className="space-y-3 px-6 pt-6">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 id="gateway-title" className="font-semibold">
              {t('gatewaySetup')}
            </h2>
            <Status
              kind={
                ready
                  ? 'success'
                  : status === 'needs_attention'
                    ? 'warning'
                    : 'neutral'
              }
            >
              {t(statusKey)}
            </Status>
          </div>
          <p className="text-sm leading-6 text-muted-foreground">
            {t(
              ready
                ? 'gatewayReadyDescription'
                : status === 'needs_attention'
                  ? 'gatewayAttentionDescription'
                  : needsKey
                    ? 'gatewayKeyNeededDescription'
                    : 'gatewaySetupDescription',
            )}
          </p>
        </div>
        <ol className="mx-6 my-3 divide-y divide-border">
          {steps.map((step, index) => (
            <li key={step.title} className="flex gap-4 py-5">
              <span
                className="flex size-6 shrink-0 items-center justify-center text-xs text-muted-foreground tabular-nums"
                aria-hidden="true"
              >
                {step.status === 'ready' ? (
                  <CircleCheck className="size-5 text-success" />
                ) : (
                  index + 1
                )}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h3 className="text-sm font-medium">{t(step.title)}</h3>
                  <span className="text-xs text-muted-foreground">
                    {t(step.status)}
                  </span>
                </div>
                <p className="mt-1.5 text-sm leading-6 text-muted-foreground">
                  {t(step.description)}
                </p>
              </div>
            </li>
          ))}
        </ol>
        <div className="border-t border-border px-6 py-4">
          <Button asChild variant="outline">
            <Link to={configured ? '/keys' : '/accounts'}>
              {t(
                configured
                  ? hasUsableKey
                    ? 'manageKeys'
                    : 'createKeyTitle'
                  : 'viewAccounts',
              )}
              <ArrowUpRight aria-hidden="true" />
            </Link>
          </Button>
        </div>
      </section>
      <section aria-labelledby="client-title" className="py-2">
        <h2 id="client-title" className="font-semibold">
          {t('clientAccess')}
        </h2>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {t('clientAccessDescription')}
        </p>
        <ul className="mt-4 flex flex-wrap gap-x-6 gap-y-3 text-sm text-muted-foreground">
          <li className="flex items-center gap-2">
            <Terminal className="size-4" aria-hidden="true" />
            {t('codexCLI')}
          </li>
          <li className="flex items-center gap-2">
            <Monitor className="size-4" aria-hidden="true" />
            {t('codexDesktop')}
          </li>
        </ul>
      </section>
    </div>
  )
}
