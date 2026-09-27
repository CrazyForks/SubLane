import { useState, type ChangeEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, LoaderCircle, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { authKey, authOptions, type AuthState } from '@/lib/auth'
import {
  backupOptions,
  backupErrorKey,
  downloadBackup,
  exportBackup,
  restoreBackup,
  verifyBackup,
  type BackupInfo,
  type BackupSettings,
  type RestoredBackup,
} from '@/lib/backup'
import { useAdminMutation } from '@/hooks/use-admin-mutation'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Status } from '@/components/Status'
import { useTimeZone } from '@/lib/timezone'

export function Backup() {
  const { data } = useQuery(authOptions())
  return data?.user?.role === 'admin' ? (
    <BackupPage key={data.user.id} userID={data.user.id} />
  ) : null
}
function BackupPage({ userID }: { userID: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(backupOptions(client, userID))
  return (
    <section
      className="mt-10 border-t border-border pt-6"
      aria-labelledby="backup-title"
    >
      <h2 id="backup-title" className="font-medium">
        {t('backupTitle')}
      </h2>
      {query.isPending ? (
        <p role="status" className="mt-6 text-sm text-muted-foreground">
          {t('loading')}
        </p>
      ) : query.data ? (
        <BackupForm settings={query.data} userID={userID} />
      ) : (
        <div role="alert" className="mt-6 space-y-3">
          <p className="text-sm text-error">{t('backupLoadFailed')}</p>
          <Button variant="outline" onClick={() => query.refetch()}>
            {t('reconnect')}
          </Button>
        </div>
      )}
    </section>
  )
}
function BackupForm({
  settings,
  userID,
}: {
  settings: BackupSettings
  userID: number
}) {
  const { t, i18n } = useTranslation()
  const timeZone = useTimeZone()
  const client = useQueryClient()
  const [file, setFile] = useState<File | null>(null)
  const [verified, setVerified] = useState<BackupInfo | null>(null)
  const [restored, setRestored] = useState<RestoredBackup | null>(null)
  const [tooLarge, setTooLarge] = useState(false)
  const current = () => {
    const user = client.getQueryData<AuthState>(authKey)?.user
    return user?.id === userID && user.role === 'admin'
  }
  const download = useAdminMutation({
    userID,
    mutationFn: (_input: void, signal) => exportBackup(signal),
    onSuccess: (response) =>
      downloadBackup(response, settings.max_upload_bytes, current),
  })
  const verify = useAdminMutation({
    userID,
    mutationFn: verifyBackup,
    onSuccess: setVerified,
  })
  const restore = useAdminMutation({
    userID,
    mutationFn: restoreBackup,
    onSuccess: async (value) => {
      setRestored(value)
      await client.invalidateQueries({ queryKey: ['backup-settings', userID] })
    },
    onError: () =>
      client.invalidateQueries({ queryKey: ['backup-settings', userID] }),
  })
  const busy = download.isPending || verify.isPending || restore.isPending
  const changed = (event: ChangeEvent<HTMLInputElement>) => {
    const next = event.target.files?.[0] ?? null
    setFile(next)
    setVerified(null)
    setTooLarge(Boolean(next && next.size > settings.max_upload_bytes))
    verify.reset()
    restore.reset()
  }
  const date = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    timeZone,
    dateStyle: 'medium',
    timeStyle: 'short',
  })
  const number = new Intl.NumberFormat(i18n.resolvedLanguage ?? 'en', {
    maximumFractionDigits: 1,
  })
  const error = download.error ?? verify.error ?? restore.error
  return (
    <div className="mt-8 space-y-8">
      <section className="flex flex-wrap items-center justify-between gap-4">
        <div className="max-w-lg">
          <h3 className="text-sm font-medium">{t('backupExportTitle')}</h3>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {t('backupExportDescription')}
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => download.mutate()}
          disabled={busy}
        >
          {download.isPending ? (
            <LoaderCircle
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          ) : (
            <Download aria-hidden="true" />
          )}
          {t(download.isPending ? 'backupExporting' : 'backupDownload')}
        </Button>
      </section>
      <section className="space-y-5 border-t border-border pt-6">
        <div>
          <h3 className="text-sm font-medium">{t('backupRestoreTitle')}</h3>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {t('backupRestoreDescription')}
          </p>
        </div>
        <div className="space-y-2">
          <label htmlFor="backup-file" className="text-sm font-medium">
            {t('backupFile')}
          </label>
          <Input
            id="backup-file"
            type="file"
            accept=".gz,.tgz"
            disabled={busy}
            onChange={changed}
            aria-describedby="backup-file-limit"
            aria-invalid={tooLarge}
          />
          <p
            id="backup-file-limit"
            className="text-xs leading-5 text-muted-foreground"
          >
            {t('backupFileLimit', {
              size: number.format(settings.max_upload_bytes / 1048576),
            })}
          </p>
        </div>
        <div className="flex flex-wrap gap-3">
          <Button
            variant="outline"
            disabled={!file || tooLarge || busy}
            onClick={() => {
              if (file) {
                setVerified(null)
                verify.mutate(file)
              }
            }}
          >
            {verify.isPending ? (
              <LoaderCircle
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : (
              <ShieldCheck aria-hidden="true" />
            )}
            {t(verify.isPending ? 'backupVerifying' : 'backupVerify')}
          </Button>
          <Button
            disabled={
              !file ||
              !verified ||
              busy ||
              settings.destination_exists ||
              restored !== null
            }
            onClick={() => {
              if (file && verified) restore.mutate(file)
            }}
          >
            {restore.isPending && (
              <LoaderCircle
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            )}
            {t(restore.isPending ? 'backupRestoring' : 'backupRestore')}
          </Button>
        </div>
        {verified && (
          <div className="space-y-4 rounded-lg bg-muted/50 p-4">
            <Status kind="success">{t('backupVerified')}</Status>
            <dl className="grid gap-4 text-sm sm:grid-cols-3">
              <div>
                <dt className="text-xs text-muted-foreground">
                  {t('backupCreatedAt')}
                </dt>
                <dd className="mt-1">
                  {date.format(new Date(verified.created_at))}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-muted-foreground">
                  {t('backupVersion')}
                </dt>
                <dd className="mt-1 break-all font-mono">{verified.version}</dd>
              </div>
              <div>
                <dt className="text-xs text-muted-foreground">
                  {t('backupDatabaseSize')}
                </dt>
                <dd className="mt-1 tabular-nums">
                  {number.format(verified.database_bytes / 1048576)} MiB
                </dd>
              </div>
            </dl>
          </div>
        )}
        {settings.destination_exists && !restored && (
          <div role="status" className="space-y-2 text-sm text-warning">
            <p>{t('backupDestinationExists')}</p>
            <code className="block break-all text-xs text-foreground">
              {settings.restore_directory}
            </code>
          </div>
        )}
        {restored && (
          <div
            role="status"
            className="space-y-3 rounded-lg border border-border p-4 text-sm"
          >
            <h3 className="font-medium">{t('backupRestored')}</h3>
            <code className="block break-all rounded-md bg-muted px-3 py-2 text-xs">
              {restored.restore_directory}
            </code>
            <ol className="list-decimal space-y-1.5 pl-5 text-muted-foreground">
              <li>{t('backupRestartStop')}</li>
              <li>{t('backupRestartDirectory')}</li>
              <li>{t('backupRestartStart')}</li>
            </ol>
            <p className="text-xs leading-5 text-muted-foreground">
              {t('backupSessionNotice')}
            </p>
          </div>
        )}
      </section>
      {(tooLarge || error) && (
        <p role="alert" className="text-sm text-error">
          {t(tooLarge ? 'backupTooLarge' : backupErrorKey(error!))}
        </p>
      )}
    </div>
  )
}
