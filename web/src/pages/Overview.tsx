import { useQuery } from '@tanstack/react-query'
import { CircleAlert, LoaderCircle, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getSystem } from '@/lib/api'
import { formatDuration } from '@/lib/duration'
import { Button } from '@/components/ui/Button'
import { Status } from '@/components/Status'
import { GatewaySetup } from '@/components/GatewaySetup'
import { useTimeZone } from '@/lib/timezone'

export function Overview() {
  const { t, i18n } = useTranslation()
  const timeZone = useTimeZone()
  const query = useQuery({
    queryKey: ['system'],
    queryFn: ({ signal }) => getSystem(signal),
  })
  const locale = i18n.resolvedLanguage ?? 'en'
  const data = query.data
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="page-title">{t('instanceStatus')}</h1>
        </div>
        <Button
          variant="outline"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RefreshCw
            className={query.isFetching ? 'motion-safe:animate-spin' : ''}
            aria-hidden="true"
          />
          {t(query.isFetching ? 'refreshing' : 'refresh')}
        </Button>
      </div>
      {query.isPending && (
        <div
          role="status"
          className="flex min-h-48 items-center justify-center gap-3 text-sm text-muted-foreground"
        >
          <LoaderCircle
            className="size-4 motion-safe:animate-spin"
            aria-hidden="true"
          />
          {t('loading')}
        </div>
      )}
      {query.isError && (
        <div
          role="alert"
          className="rounded-xl border border-border bg-card p-6"
        >
          <div className="flex items-center gap-2 font-medium text-error">
            <CircleAlert className="size-4" aria-hidden="true" />
            {t('connectionError')}
          </div>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {t(
              data
                ? 'refreshFailed'
                : query.error.message === 'invalid_response'
                  ? 'invalidResponse'
                  : 'unavailable',
            )}
          </p>
          {!data && (
            <Button
              className="mt-4"
              variant="outline"
              onClick={() => query.refetch()}
              disabled={query.isFetching}
            >
              {t('reconnect')}
            </Button>
          )}
        </div>
      )}
      {data && (
        <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(16rem,1fr)]">
          <GatewaySetup
            status={query.isError ? 'unknown' : data.gateway.status}
            hasUsableKey={query.isError ? null : data.gateway.has_usable_key}
          />
          <section
            aria-labelledby="service-title"
            className="rounded-xl border border-border bg-card"
          >
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-6 py-5">
              <h2 id="service-title" className="font-semibold">
                {t('service')}
              </h2>
              {/* Cached readings stay visible after a failed refresh, but must not imply live health. */}
              <Status kind={query.isError ? 'warning' : 'success'}>
                {t(query.isError ? 'statusUnknown' : 'running')}
              </Status>
            </div>
            <dl className="space-y-5 p-6 text-sm">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <dt className="text-muted-foreground">{t('version')}</dt>
                <dd className="break-all font-medium">{data.version}</dd>
              </div>
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <dt className="text-muted-foreground">{t('uptime')}</dt>
                <dd className="font-medium tabular-nums">
                  {formatDuration(data.uptime_seconds, locale)}
                </dd>
              </div>
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <dt className="text-muted-foreground">{t('storage')}</dt>
                <dd className="font-medium">
                  SQLite{' '}
                  <span className="text-muted-foreground">/ {t('ready')}</span>
                </dd>
              </div>
            </dl>
            <p className="flex flex-wrap items-center justify-between gap-2 border-t border-border px-6 py-4 text-xs text-muted-foreground">
              {t('lastChecked')}
              <time
                dateTime={new Date(query.dataUpdatedAt).toISOString()}
                className="tabular-nums"
              >
                {new Intl.DateTimeFormat(locale, {
                  timeZone,
                  hour: '2-digit',
                  minute: '2-digit',
                  second: '2-digit',
                }).format(query.dataUpdatedAt)}
              </time>
            </p>
          </section>
        </div>
      )}
    </div>
  )
}
