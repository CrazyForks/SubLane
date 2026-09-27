import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { List, LoaderCircle, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  catalogOptions,
  refreshCatalog,
  type CatalogTarget,
} from '@/lib/catalog'
import { authKey, type AuthState } from '@/lib/auth'
import { useAdminMutation } from '@/hooks/use-admin-mutation'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './ui/Dialog'

import { useTimeZone } from '@/lib/timezone'
export function CatalogDialog({
  target,
  name,
  disabled = false,
}: {
  target: CatalogTarget
  name: string
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          disabled={disabled}
          aria-label={t('catalogFor', { name })}
        >
          <List aria-hidden="true" />
          {t('catalogModels')}
        </Button>
      </DialogTrigger>
      {open && <CatalogContent target={target} name={name} />}
    </Dialog>
  )
}
function CatalogContent({
  target,
  name,
}: {
  target: CatalogTarget
  name: string
}) {
  const { t, i18n } = useTranslation()
  const timeZone = useTimeZone()
  const client = useQueryClient()
  const options = catalogOptions(client, target)
  const query = useQuery(options)
  const [filter, setFilter] = useState('')
  const [now, setNow] = useState(Date.now)
  const [ownerID] = useState(
    () => client.getQueryData<AuthState>(authKey)?.user?.id,
  )
  const refresh = useAdminMutation({
    userID: ownerID ?? 0,
    mutationFn: (_input: void, signal) =>
      refreshCatalog(String(target.id), signal),
    onSuccess: (data) => client.setQueryData(options.queryKey, data),
  })
  const data = query.data
  const cooldown = Math.max(
    0,
    (data?.retry_after_seconds ?? 0) -
      Math.floor(Math.max(0, now - query.dataUpdatedAt) / 1000),
  )
  useEffect(() => {
    if (!cooldown) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [cooldown])
  const pending =
    query.isFetching || refresh.isPending || Boolean(data?.refreshing)
  const failed =
    query.isError || refresh.isError || Boolean(data?.refresh_failed)
  const models = (data?.models ?? []).filter((model) =>
    model.toLowerCase().includes(filter.toLowerCase()),
  )
  return (
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
      <DialogHeader>
        <DialogTitle>{t('catalogFor', { name })}</DialogTitle>
        <DialogDescription>
          {t(
            target.kind === 'account'
              ? 'catalogAccountDescription'
              : 'catalogGroupDescription',
          )}
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
        <span>
          {data?.known
            ? t('catalogCount', { count: data.models.length })
            : t('catalogUnknown')}
        </span>
        <Button
          variant="outline"
          size="sm"
          disabled={pending || (target.kind === 'account' && cooldown > 0)}
          onClick={() =>
            target.kind === 'account' ? refresh.mutate() : query.refetch()
          }
        >
          <RefreshCw
            aria-hidden="true"
            className={pending ? 'motion-safe:animate-spin' : undefined}
          />
          {t(target.kind === 'account' ? 'catalogRefresh' : 'catalogReload')}
        </Button>
      </div>
      {query.isPending ? (
        <p
          role="status"
          className="flex items-center gap-2 text-sm text-muted-foreground"
        >
          <LoaderCircle
            aria-hidden="true"
            className="size-4 motion-safe:animate-spin"
          />
          {t('catalogLoading')}
        </p>
      ) : (
        <>
          {failed && (
            <p role="status" className="text-sm text-warning">
              {t(data?.known ? 'catalogRefreshFailed' : 'catalogLoadFailed')}
            </p>
          )}
          {data?.refreshing && (
            <p role="status" className="text-sm text-muted-foreground">
              {t('catalogRefreshing')}
            </p>
          )}
          {data?.partial && (
            <p role="status" className="text-sm text-warning">
              {t('catalogPartial')}
            </p>
          )}
          {data?.known && data.stale && (!data.usable || !failed) && (
            <p className="text-sm text-muted-foreground">
              {t(data.usable ? 'catalogStale' : 'catalogExpired')}
            </p>
          )}
          {data && !data.known && (
            <p className="text-sm text-muted-foreground">
              {t('catalogUnknownHint')}
            </p>
          )}
          {data?.known && data.models.length === 0 && (
            <p className="py-4 text-sm text-muted-foreground">
              {t(
                target.kind === 'account'
                  ? 'catalogEmptyAccount'
                  : 'catalogEmptyGroup',
              )}
            </p>
          )}
          {Boolean(data?.models.length) && (
            <>
              <Input
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                placeholder={t('catalogSearch')}
                aria-label={t('catalogSearch')}
              />
              <ul
                className="max-h-72 overflow-y-auto divide-y divide-border"
                aria-label={t('catalogAvailableModels')}
              >
                {models.slice(0, 200).map((model) => (
                  <li
                    key={model}
                    className="break-all py-2.5 font-mono text-xs"
                  >
                    {model}
                  </li>
                ))}
              </ul>
              {!models.length && (
                <p className="text-sm text-muted-foreground">
                  {t('catalogNoMatch')}
                </p>
              )}
              {models.length > 200 && (
                <p className="text-xs text-muted-foreground">
                  {t('catalogNarrowSearch')}
                </p>
              )}
            </>
          )}
          {Boolean(data?.updated_at) && (
            <p className="text-xs text-muted-foreground">
              {t('catalogUpdated', {
                time: new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
                  timeZone,
                  dateStyle: 'short',
                  timeStyle: 'short',
                }).format(data!.updated_at * 1000),
              })}
            </p>
          )}
        </>
      )}
    </DialogContent>
  )
}
