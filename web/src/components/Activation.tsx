import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { setupOrder, setupSteps, type SetupProgress } from '@/lib/activation'
import { connectionOptions } from '@/lib/connection'
import { useTour } from '@/lib/tour'
import { Button } from './ui/Button'

export function Activation({
  setup,
  administrator = false,
}: {
  setup: SetupProgress
  administrator?: boolean
}) {
  const { t } = useTranslation()
  const tour = useTour()
  const steps = setupOrder(administrator)
  const current =
    setup.stage === 'complete'
      ? steps.length
      : steps.findIndex((step) => step === setup.stage)
  const next = setupSteps[setup.stage]
  return (
    <section
      className="rounded-xl border border-border bg-card p-6"
      aria-label={t('activationTitle')}
    >
      <h2 className="font-semibold">
        {t(
          setup.stage === 'complete' ? 'activationComplete' : 'activationTitle',
        )}
      </h2>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">
        {t(next.hint)}
      </p>
      {setup.stage !== 'complete' && (
        <p className="mt-3 text-xs text-muted-foreground">
          {t('tourStep', {
            current: Math.max(0, current) + 1,
            total: steps.length,
          })}{' '}
          · {t(next.title)}
        </p>
      )}
      <div className="mt-5 flex flex-wrap gap-3">
        {tour && setup.stage !== 'complete' && (
          <Button onClick={() => tour.move(setup.stage)}>
            {t('tourStart')}
          </Button>
        )}
        <Button asChild variant="outline">
          <Link to={next.to}>{t(next.action)}</Link>
        </Button>
      </div>
    </section>
  )
}

export function PersonalActivation({ userID }: { userID: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(connectionOptions(client, userID))
  if (query.isPending)
    return (
      <p role="status" className="text-sm text-muted-foreground">
        {t('gatewayChecking')}
      </p>
    )
  if (query.isError)
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t('connectionStatusError')}
        </p>
        <Button variant="outline" onClick={() => query.refetch()}>
          {t('refresh')}
        </Button>
      </div>
    )
  if (query.data.setup && query.data.setup.stage !== 'complete')
    return <Activation setup={query.data.setup} />
  return (
    <section>
      <h2 className="font-semibold">{t('clientAccess')}</h2>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">
        {t('memberAccessDescription')}
      </p>
      <Button asChild variant="outline" className="mt-4">
        <Link to="/keys">{t('manageKeys')}</Link>
      </Button>
    </section>
  )
}
