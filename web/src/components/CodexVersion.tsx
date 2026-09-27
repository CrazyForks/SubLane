import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAdminMutation } from '@/hooks/use-admin-mutation'
import {
  saveVersion,
  syncVersion,
  validVersion,
  versionErrorKey,
  versionOptions,
  type VersionState,
  type VersionConfig,
} from '@/lib/version'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { useTimeZone } from '@/lib/timezone'

export function CodexVersion({ userID }: { userID: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(versionOptions(client, userID))
  return (
    <section
      className="mt-10 border-t border-border pt-6"
      aria-labelledby="codex-version-title"
    >
      <h2 id="codex-version-title" className="font-medium">
        {t('codexVersionTitle')}
      </h2>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">
        {t('codexVersionDescription')}
      </p>
      {query.isPending ? (
        <p role="status" className="mt-5 text-sm text-muted-foreground">
          {t('loading')}
        </p>
      ) : query.data ? (
        <VersionForm
          value={query.data}
          userID={userID}
          updatedAt={query.dataUpdatedAt}
        />
      ) : (
        <div role="alert" className="mt-5 space-y-3">
          <p className="text-sm text-error">{t('codexVersionLoadFailed')}</p>
          <Button variant="outline" onClick={() => query.refetch()}>
            {t('reconnect')}
          </Button>
        </div>
      )}
    </section>
  )
}
function VersionForm({
  value,
  userID,
  updatedAt,
}: {
  value: VersionState
  userID: number
  updatedAt: number
}) {
  const { t, i18n } = useTranslation()
  const timeZone = useTimeZone()
  const client = useQueryClient()
  const options = versionOptions(client, userID)
  const [manual, setManual] = useState(value.manual_version)
  const [auto, setAuto] = useState(value.auto_sync)
  const [invalid, setInvalid] = useState(false)
  const [saved, setSaved] = useState(false)
  const [now, setNow] = useState(Date.now)
  const cooldown = Math.max(
    0,
    value.retry_after_seconds - Math.floor(Math.max(0, now - updatedAt) / 1000),
  )
  useEffect(() => {
    if (!cooldown) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [cooldown])
  const apply = async (data: VersionState) => {
    const previous = client.getQueryData<VersionState>(options.queryKey)
    client.setQueryData(options.queryKey, data)
    if (previous?.effective_version !== data.effective_version)
      await client.invalidateQueries({ queryKey: ['model-catalog'] })
  }
  const save = useAdminMutation<VersionState, VersionConfig>({
    userID,
    mutationFn: saveVersion,
    onSuccess: async (data) => {
      await apply(data)
      setManual(data.manual_version)
      setAuto(data.auto_sync)
      setSaved(true)
    },
  })
  const sync = useAdminMutation({
    userID,
    mutationFn: (_input: void, signal) => syncVersion(signal),
    onSuccess: apply,
    onError: () => client.invalidateQueries({ queryKey: options.queryKey }),
  })
  const error = save.error ?? sync.error
  const sourceLabels = {
    manual: 'codexVersionManualSource',
    synced: 'codexVersionSyncedSource',
    builtin: 'codexVersionBuiltinSource',
  } as const
  const date = (timestamp: number) =>
    new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
      timeZone,
      dateStyle: 'short',
      timeStyle: 'short',
    }).format(timestamp * 1000)
  const dirty =
    manual.trim() !== value.manual_version || auto !== value.auto_sync
  const edit = () => {
    setSaved(false)
    setInvalid(false)
    save.reset()
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (save.isPending) return
    setInvalid(!validVersion(manual))
    if (validVersion(manual))
      save.mutate({ manual_version: manual.trim(), auto_sync: auto })
  }
  return (
    <div className="mt-6 space-y-6">
      <dl className="grid gap-5 rounded-lg bg-muted/50 p-4 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-xs text-muted-foreground">
            {t('codexVersionEffective')}
          </dt>
          <dd className="mt-2 flex flex-wrap items-center gap-2">
            <code className="font-medium tabular-nums">
              {value.effective_version}
            </code>
            <span className="text-xs text-muted-foreground">
              {t(sourceLabels[value.source])}
            </span>
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">
            {t('codexVersionLatest')}
          </dt>
          <dd className="mt-2 font-mono tabular-nums">
            {value.latest_version || '—'}
          </dd>
        </div>
      </dl>
      <form onSubmit={submit} noValidate className="space-y-5">
        <div className="max-w-sm space-y-2">
          <label htmlFor="codex-manual-version" className="text-sm font-medium">
            {t('codexVersionManual')}
          </label>
          <Input
            id="codex-manual-version"
            value={manual}
            onChange={(event) => {
              setManual(event.target.value)
              edit()
            }}
            placeholder={t('codexVersionAutomatic', {
              version: value.default_version,
            })}
            maxLength={24}
            disabled={save.isPending}
            autoComplete="off"
            spellCheck={false}
            aria-invalid={invalid}
            aria-describedby="codex-version-hint"
          />
          <p
            id="codex-version-hint"
            className="text-xs leading-5 text-muted-foreground"
          >
            {t('codexVersionManualHint')}
          </p>
        </div>
        <label className="flex items-start gap-3">
          <input
            type="checkbox"
            aria-labelledby="codex-version-auto-label"
            aria-describedby="codex-version-auto-hint"
            checked={auto}
            onChange={(event) => {
              setAuto(event.target.checked)
              edit()
            }}
            disabled={save.isPending}
            className="mt-0.5 size-4 shrink-0 accent-primary"
          />
          <span>
            <span id="codex-version-auto-label" className="text-sm font-medium">
              {t('codexVersionAuto')}
            </span>
            <span
              id="codex-version-auto-hint"
              className="mt-1 block text-xs leading-5 text-muted-foreground"
            >
              {t('codexVersionAutoHint')}
            </span>
          </span>
        </label>
        {invalid && (
          <p role="alert" className="text-sm text-error">
            {t('codexVersionInvalid')}
          </p>
        )}
        {!invalid && error && (
          <p role="alert" className="text-sm text-error">
            {t(versionErrorKey(error))}
          </p>
        )}
        {saved && (
          <p role="status" className="text-sm text-success">
            {t('codexVersionSaved')}
          </p>
        )}
        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" disabled={save.isPending || !dirty}>
            {save.isPending && (
              <LoaderCircle
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            )}
            {t('codexVersionSave')}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={sync.isPending || value.syncing || cooldown > 0}
            onClick={() => {
              sync.mutate()
              setSaved(false)
            }}
          >
            <RefreshCw
              aria-hidden="true"
              className={
                sync.isPending || value.syncing
                  ? 'motion-safe:animate-spin'
                  : undefined
              }
            />
            {t('codexVersionCheck')}
          </Button>
        </div>
      </form>
      <div className="space-y-1.5 text-xs leading-5 text-muted-foreground">
        {value.syncing ? (
          <p role="status">{t('codexVersionChecking')}</p>
        ) : value.sync_failed && !error ? (
          <p role="status" className="text-warning">
            {t('codexVersionSyncFailed')}
          </p>
        ) : null}
        <p>
          {value.checked_at
            ? t('codexVersionChecked', { time: date(value.checked_at) })
            : t('codexVersionNeverChecked')}
        </p>
        {value.auto_sync && value.next_check_at > 0 && (
          <p>
            {t('codexVersionNextCheck', { time: date(value.next_check_at) })}
          </p>
        )}
        {!value.auto_sync && <p>{t('codexVersionAutoOff')}</p>}
      </div>
    </div>
  )
}
