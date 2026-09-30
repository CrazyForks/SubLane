import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  availabilityOptions,
  availabilityReasons,
  type AvailabilityTarget,
} from '@/lib/availability'
import { formatInstanceDate, useTimeZone } from '@/lib/timezone'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { Status } from './Status'

export function ModelAvailability({
  target,
  model,
  onInspect,
}: {
  target: AvailabilityTarget
  model: string
  onInspect: (model: string) => void
}) {
  const { t, i18n } = useTranslation()
  const zone = useTimeZone()
  const client = useQueryClient()
  const query = useQuery(availabilityOptions(client, target, model))
  const [draft, setDraft] = useState(model)
  const data = query.data
  const date = (value: number) =>
    formatInstanceDate(value * 1000, i18n.resolvedLanguage ?? 'en', zone, {
      dateStyle: 'short',
      timeStyle: 'medium',
    })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const requested = draft.trim()
    if (requested === model) return query.refetch()
    onInspect(requested)
  }
  return (
    <section
      className="space-y-3 border-t border-border pt-4"
      aria-label={t('availabilityTitle')}
    >
      <h3 className="text-sm font-medium">{t('availabilityTitle')}</h3>
      <form onSubmit={submit} className="flex flex-wrap items-end gap-2">
        <div className="min-w-0 flex-1 space-y-2">
          <label
            htmlFor="availability-model"
            className="text-xs text-muted-foreground"
          >
            {t('availabilityModel')}
          </label>
          <Input
            id="availability-model"
            value={draft}
            maxLength={128}
            onChange={(event) => setDraft(event.target.value)}
          />
        </div>
        <Button variant="outline" disabled={!draft.trim() || query.isFetching}>
          {t(query.isFetching ? 'availabilityChecking' : 'availabilityCheck')}
        </Button>
      </form>
      <p className="text-xs leading-5 text-muted-foreground">
        {t('availabilityHint')}
      </p>
      {model && query.isPending && (
        <p role="status" className="text-sm text-muted-foreground">
          {t('availabilityChecking')}
        </p>
      )}
      {query.isError && (
        <p role="alert" className="text-sm text-error">
          {t('availabilityFailed')}
        </p>
      )}
      {data && (
        <div className="space-y-3" aria-live="polite">
          <div className="flex flex-wrap items-center gap-2">
            <span className="break-all font-mono text-xs">{data.model}</span>
            <Status
              kind={
                data.state === 'available'
                  ? 'success'
                  : data.state === 'unknown'
                    ? 'neutral'
                    : 'warning'
              }
            >
              {t(
                data.state === 'available'
                  ? 'availabilityEligible'
                  : data.state === 'unknown'
                    ? 'availabilityUnknown'
                    : 'availabilityUnavailable',
              )}
            </Status>
          </div>
          <p className="text-sm">
            {t('availabilitySummary', {
              available: data.available,
              unknown: data.unknown,
            })}
          </p>
          {data.retry_at > data.server_time && (
            <p className="text-xs text-muted-foreground">
              {t('availabilityRetryAt', { date: date(data.retry_at) })}
            </p>
          )}
          {data.reasons.length > 0 && (
            <ul className="space-y-1 text-xs text-muted-foreground">
              {data.reasons.map((reason) => (
                <li key={reason.code}>
                  {t(availabilityReasons[reason.code] ?? 'reasonUnknown')} ·{' '}
                  {reason.count}
                </li>
              ))}
            </ul>
          )}
          {target.kind === 'group' && Boolean(data.accounts?.length) && (
            <ul
              className="max-h-48 divide-y divide-border overflow-y-auto text-xs"
              aria-label={t('availabilityAccounts')}
            >
              {data.accounts!.map((account) => (
                <li key={account.id} className="space-y-1 py-2">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="break-all font-medium">
                      {account.name}
                    </span>
                    <span className="text-muted-foreground">
                      {t(
                        availabilityReasons[account.reason] ?? 'reasonUnknown',
                      )}
                    </span>
                  </div>
                  <p className="text-muted-foreground">
                    {account.provider} ·{' '}
                    {t('availabilityConcurrency', {
                      active: account.in_flight,
                      limit: account.max_concurrency,
                    })}
                    {account.quota_used_percent !== undefined
                      ? ` · ${t('availabilityQuotaUsed', { percent: account.quota_used_percent })}`
                      : ` · ${t('availabilityQuotaUnknown')}`}
                  </p>
                  {account.retry_at > data.server_time && (
                    <p className="text-muted-foreground">
                      {t('availabilityRetryAt', {
                        date: date(account.retry_at),
                      })}
                    </p>
                  )}
                </li>
              ))}
            </ul>
          )}
          <p className="text-xs text-muted-foreground">
            {t('availabilityObserved', { date: date(data.server_time) })}
          </p>
        </div>
      )}
    </section>
  )
}
