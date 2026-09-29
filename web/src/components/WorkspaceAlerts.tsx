import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  alertOptions,
  saveAlerts,
  alertNames,
  type AlertState,
  type AlertInput,
} from '@/lib/alerts'
import { useAdminMutation } from '@/hooks/use-admin-mutation'
import { useTimeZone, formatInstanceDate } from '@/lib/timezone'
import { ApiError } from '@/lib/request'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { Status } from './Status'

export function WorkspaceAlerts({ userID }: { userID: number }) {
  const { t, i18n } = useTranslation()
  const zone = useTimeZone()
  const query = useQuery(alertOptions(userID))
  const [editing, setEditing] = useState(false)
  const value = query.data
  return (
    <section
      className="space-y-4 border-y border-border py-5"
      aria-label={t('alertsTitle')}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h2 className="font-semibold">{t('alertsTitle')}</h2>
          {value && !query.isError && (
            <Status
              kind={
                value.delivery_failed || value.incidents.length
                  ? 'warning'
                  : 'neutral'
              }
            >
              {t(value.enabled ? 'alertsEnabled' : 'alertsDisabled')}
            </Status>
          )}
        </div>
        <Button
          variant="outline"
          disabled={!value || query.isError}
          onClick={() => setEditing(!editing)}
        >
          {t(editing ? 'cancel' : 'alertsConfigure')}
        </Button>
      </div>
      {query.isPending ? (
        <p className="text-sm text-muted-foreground">{t('loading')}</p>
      ) : query.isError ? (
        <div className="flex items-center gap-3">
          <p className="text-sm text-error">{t('alertsUnavailable')}</p>
          <Button variant="ghost" onClick={() => query.refetch()}>
            {t('refresh')}
          </Button>
        </div>
      ) : (
        value && (
          <>
            {!value.enabled && (
              <p className="text-sm leading-6 text-muted-foreground">
                {t('alertsDescription')}
              </p>
            )}
            {value.configured && (
              <p className="text-sm text-muted-foreground">
                {value.destination}
              </p>
            )}
            {value.delivery_failed && (
              <p role="alert" className="text-sm text-error">
                {t('alertsDeliveryFailed')}
              </p>
            )}
            {value.enabled && value.last_delivered_at === 0 && (
              <p className="text-sm text-muted-foreground">
                {t('alertsNeverDelivered')}
              </p>
            )}
            {value.last_delivered_at > 0 && (
              <p className="text-xs text-muted-foreground">
                {t('alertsLastDelivered', {
                  date: formatInstanceDate(
                    value.last_delivered_at * 1000,
                    i18n.resolvedLanguage ?? 'en',
                    zone,
                    { dateStyle: 'short', timeStyle: 'short' },
                  ),
                })}
              </p>
            )}
            {value.incidents.length > 0 && (
              <ul className="divide-y divide-border text-sm">
                {value.incidents.map((incident) => (
                  <li
                    key={`${incident.kind}:${incident.subject}`}
                    className="py-2"
                  >
                    {t(alertNames[incident.kind])} ·{' '}
                    <span className="break-all text-muted-foreground">
                      {incident.subject}
                    </span>
                  </li>
                ))}
              </ul>
            )}
            {editing && (
              <AlertForm
                key={userID}
                userID={userID}
                value={value}
                onSaved={() => setEditing(false)}
              />
            )}
          </>
        )
      )}
    </section>
  )
}

function AlertForm({
  userID,
  value,
  onSaved,
}: {
  userID: number
  value: AlertState
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [enabled, setEnabled] = useState(value.enabled)
  const [url, setURL] = useState('')
  const [clear, setClear] = useState(false)
  const mutation = useAdminMutation<AlertState, AlertInput>({
    userID,
    mutationFn: saveAlerts,
    onSuccess: async (result) => {
      setURL('')
      client.setQueryData(alertOptions(userID).queryKey, result)
      onSaved()
      await client.invalidateQueries({ queryKey: ['audit'] })
    },
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    mutation.mutate({ enabled, url, clear })
  }
  return (
    <form
      className="max-w-xl space-y-4 border-t border-border pt-4"
      onSubmit={submit}
    >
      <p className="text-sm leading-6 text-muted-foreground">
        {t('alertsPayload')}
      </p>
      <label className="flex items-center gap-3 text-sm">
        <input
          type="checkbox"
          checked={enabled}
          disabled={clear || mutation.isPending}
          onChange={(event) => setEnabled(event.target.checked)}
        />
        {t('alertsEnable')}
      </label>
      <div className="space-y-2">
        <label htmlFor="webhook-url" className="text-sm font-medium">
          {t('alertsURL')}
        </label>
        <Input
          id="webhook-url"
          type="url"
          autoComplete="off"
          value={url}
          maxLength={2048}
          disabled={clear || mutation.isPending}
          onChange={(event) => setURL(event.target.value)}
          placeholder="https://hooks.example.com/…"
          aria-describedby="webhook-hint"
        />
        <p
          id="webhook-hint"
          className="text-xs leading-5 text-muted-foreground"
        >
          {t(value.configured ? 'alertsKeepURL' : 'alertsURLHint')}
        </p>
      </div>
      {value.configured && (
        <label className="flex items-center gap-3 text-sm">
          <input
            type="checkbox"
            checked={clear}
            disabled={mutation.isPending}
            onChange={(event) => {
              setClear(event.target.checked)
              if (event.target.checked) {
                setEnabled(false)
                setURL('')
              }
            }}
          />
          {t('alertsClear')}
        </label>
      )}
      {mutation.isError && (
        <p role="alert" className="text-sm text-error">
          {t(
            mutation.error instanceof ApiError &&
              mutation.error.code === 'invalid_alert_settings'
              ? 'alertsInvalid'
              : 'alertsUnavailable',
          )}
        </p>
      )}
      <Button
        disabled={mutation.isPending || (enabled && !value.configured && !url)}
      >
        {t(mutation.isPending ? 'alertsSaving' : 'saveChanges')}
      </Button>
    </form>
  )
}
