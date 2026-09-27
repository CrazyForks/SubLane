import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  LoaderCircle,
  Pencil,
  Plus,
  Route,
  ScanSearch,
  Trash2,
  Upload,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ProxyEditor } from '@/components/ProxyEditor'
import { ProxyImport } from '@/components/ProxyImport'
import { Status } from '@/components/Status'
import {
  checkProxy,
  deleteProxy,
  pruneFailedProxies,
  proxyErrorKey,
  proxyOptions,
  type Proxy,
} from '@/lib/proxies'
import { Button } from '@/components/ui/Button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/Dialog'

import { useTimeZone } from '@/lib/timezone'
export function Proxies() {
  const { t, i18n } = useTranslation()
  const timeZone = useTimeZone()
  const client = useQueryClient()
  const query = useQuery(proxyOptions)
  const heading = useRef<HTMLHeadingElement>(null)
  const [editing, setEditing] = useState<Proxy | 'new' | null>(null)
  const [importing, setImporting] = useState(false)
  const [deleting, setDeleting] = useState<Proxy | null>(null)
  const [pruning, setPruning] = useState(false)
  const [prunedCount, setPrunedCount] = useState<number | null>(null)
  const [bulkProgress, setBulkProgress] = useState<{
    done: number
    total: number
  } | null>(null)
  const [bulkResult, setBulkResult] = useState<{
    total: number
    connected: number
    failed: number
    errors: number
  } | null>(null)
  const remove = useMutation({
    mutationFn: deleteProxy,
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['proxies'] })
      setDeleting(null)
    },
  })
  const check = useMutation({
    mutationFn: checkProxy,
    onSuccess: () => client.invalidateQueries({ queryKey: ['proxies'] }),
  })
  const checkAll = useMutation({
    mutationFn: async (ids: string[]) => {
      let next = 0
      let done = 0
      const result = { total: ids.length, connected: 0, failed: 0, errors: 0 }
      setBulkProgress({ done: 0, total: ids.length })
      const worker = async () => {
        while (next < ids.length) {
          const id = ids[next++]
          try {
            const proxy = await checkProxy(id)
            if (proxy.reachable) result.connected++
            else if (proxy.check_error === 'connection_failed') result.failed++
            else result.errors++
          } catch {
            result.errors++
          }
          done++
          setBulkProgress({ done, total: ids.length })
        }
      }
      await Promise.all(
        Array.from({ length: Math.min(4, ids.length) }, () => worker()),
      )
      return result
    },
    onSuccess: async (result) => {
      setBulkResult(result)
      setBulkProgress(null)
      await client.invalidateQueries({ queryKey: ['proxies'] })
    },
  })
  const prune = useMutation({
    mutationFn: pruneFailedProxies,
    onSuccess: async (result) => {
      setPrunedCount(result.deleted_count)
      setPruning(false)
      await client.invalidateQueries({ queryKey: ['proxies'] })
    },
  })
  const saved = () => client.invalidateQueries({ queryKey: ['proxies'] })
  const recentFailures =
    query.data?.proxies.filter((proxy) => proxy.prune_eligible).length ?? 0
  const countryNames = new Intl.DisplayNames(i18n.resolvedLanguage ?? 'en', {
    type: 'region',
  })
  const dates = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? 'en', {
    timeZone,
    dateStyle: 'medium',
    timeStyle: 'short',
  })
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 ref={heading} tabIndex={-1} className="page-title outline-none">
            {t('proxiesTitle')}
          </h1>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => setImporting(true)}>
            <Upload aria-hidden="true" />
            {t('importProxies')}
          </Button>
          <Button onClick={() => setEditing('new')}>
            <Plus aria-hidden="true" />
            {t('addProxy')}
          </Button>
        </div>
      </div>
      <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
        {t('proxiesUsageHint')}
      </p>
      <p className="max-w-3xl text-xs leading-5 text-muted-foreground">
        {t('proxyProbeHint')}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={
            !query.data?.proxies.length ||
            check.isPending ||
            checkAll.isPending ||
            prune.isPending
          }
          onClick={() => {
            if (!query.data) return
            setBulkResult(null)
            setPrunedCount(null)
            checkAll.mutate(query.data.proxies.map((proxy) => proxy.id))
          }}
        >
          {checkAll.isPending ? (
            <LoaderCircle
              className="motion-safe:animate-spin"
              aria-hidden="true"
            />
          ) : (
            <ScanSearch aria-hidden="true" />
          )}
          {checkAll.isPending && bulkProgress
            ? t('proxyBulkProgress', bulkProgress)
            : t('testAllProxies')}
        </Button>
        <Button
          variant="outline"
          disabled={
            recentFailures === 0 ||
            check.isPending ||
            checkAll.isPending ||
            prune.isPending
          }
          onClick={() => {
            prune.reset()
            setPruning(true)
          }}
        >
          <Trash2 aria-hidden="true" />
          {t('deleteFailedProxies', { count: recentFailures })}
        </Button>
      </div>
      {bulkResult && (
        <div role="status" className="space-y-1 text-sm">
          <p>{t('proxyBulkSummary', { count: bulkResult.total })}</p>
          <p className="text-xs text-muted-foreground">
            {t('proxyBulkDetail', bulkResult)}
          </p>
        </div>
      )}
      {prunedCount !== null && (
        <p role="status" className="text-sm text-success">
          {t('proxyPruned', { count: prunedCount })}
        </p>
      )}
      {check.isError && (
        <p role="alert" className="text-sm text-error">
          {t(proxyErrorKey(check.error))}
        </p>
      )}
      {query.isPending ? (
        <p
          role="status"
          className="flex items-center gap-2 py-10 text-sm text-muted-foreground"
        >
          <LoaderCircle
            className="size-4 motion-safe:animate-spin"
            aria-hidden="true"
          />
          {t('loadingProxies')}
        </p>
      ) : query.isError ? (
        <div
          role="alert"
          className="space-y-4 rounded-xl border border-border p-6"
        >
          <p className="text-sm text-error">{t('proxiesLoadFailed')}</p>
          <Button variant="outline" onClick={() => query.refetch()}>
            {t('reconnect')}
          </Button>
        </div>
      ) : query.data.proxies.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-6 py-12 text-center">
          <Route
            className="mx-auto mb-3 size-7 text-muted-foreground"
            aria-hidden="true"
          />
          <p className="font-medium">{t('proxiesEmptyTitle')}</p>
          <p className="mt-2 text-sm text-muted-foreground">
            {t('proxiesEmptyDescription')}
          </p>
        </div>
      ) : (
        <ul
          className="divide-y divide-border rounded-xl border border-border bg-card"
          aria-label={t('proxiesTitle')}
        >
          {query.data.proxies.map((proxy) => (
            <li
              key={proxy.id}
              className="flex flex-wrap items-center justify-between gap-4 p-5"
            >
              <div className="min-w-0 space-y-1">
                <h2 className="break-words font-medium">{proxy.name}</h2>
                <p className="break-all text-sm text-muted-foreground">
                  {proxy.endpoint}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t('proxyAccountCount', { count: proxy.account_count })}
                </p>
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                  {proxy.checked_at === 0 ? (
                    <Status kind="neutral">{t('proxyNotTested')}</Status>
                  ) : (
                    <>
                      <Status
                        kind={
                          proxy.reachable
                            ? 'success'
                            : proxy.check_error === 'connection_failed'
                              ? 'error'
                              : 'warning'
                        }
                      >
                        {t(
                          proxy.reachable
                            ? 'proxyConnected'
                            : proxy.check_error === 'lookup_rate_limited'
                              ? 'proxyLookupRateLimited'
                              : proxy.check_error === 'connection_failed'
                                ? 'proxyConnectionFailed'
                                : 'proxyLookupUnavailable',
                        )}
                      </Status>
                      <time
                        dateTime={new Date(
                          proxy.checked_at * 1000,
                        ).toISOString()}
                      >
                        {dates.format(proxy.checked_at * 1000)}
                      </time>
                      {proxy.reachable && (
                        <span>
                          {t('proxyLatency', { latency: proxy.latency_ms })}
                        </span>
                      )}
                    </>
                  )}
                </div>
                {proxy.reachable && (
                  <div className="space-y-1 text-xs text-muted-foreground">
                    <p>{t('proxyExitIP', { ip: proxy.exit_ip })}</p>
                    <p>
                      {t('proxyLocation', {
                        location:
                          [
                            proxy.city,
                            proxy.region,
                            proxy.country
                              ? (countryNames.of(proxy.country) ??
                                proxy.country)
                              : '',
                          ]
                            .filter(Boolean)
                            .join(', ') || t('proxyLocationUnknown'),
                      })}
                    </p>
                  </div>
                )}
              </div>
              <div className="flex flex-wrap gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  className="text-muted-foreground [@media(pointer:coarse)]:size-11"
                  title={t('testProxyNamed', { name: proxy.name })}
                  aria-label={t('testProxyNamed', { name: proxy.name })}
                  disabled={
                    check.isPending || checkAll.isPending || prune.isPending
                  }
                  onClick={() => check.mutate(proxy.id)}
                >
                  {check.isPending && check.variables === proxy.id ? (
                    <LoaderCircle
                      className="motion-safe:animate-spin"
                      aria-hidden="true"
                    />
                  ) : (
                    <ScanSearch aria-hidden="true" />
                  )}
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="text-muted-foreground [@media(pointer:coarse)]:size-11"
                  title={t('editProxyNamed', { name: proxy.name })}
                  aria-label={t('editProxyNamed', { name: proxy.name })}
                  onClick={() => setEditing(proxy)}
                >
                  <Pencil aria-hidden="true" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="text-error hover:text-error focus-visible:text-error [@media(pointer:coarse)]:size-11"
                  title={t('deleteProxyNamed', { name: proxy.name })}
                  aria-label={t('deleteProxyNamed', { name: proxy.name })}
                  disabled={checkAll.isPending || prune.isPending}
                  onClick={() => {
                    remove.reset()
                    setDeleting(proxy)
                  }}
                >
                  <Trash2 aria-hidden="true" />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {editing && (
        <ProxyEditor
          proxy={editing === 'new' ? undefined : editing}
          onClose={() => setEditing(null)}
          onSaved={saved}
          restoreFocus={() => heading.current?.focus()}
        />
      )}
      {importing && (
        <ProxyImport
          onClose={() => setImporting(false)}
          onImported={saved}
          restoreFocus={() => heading.current?.focus()}
        />
      )}
      {deleting && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !remove.isPending) setDeleting(null)
          }}
        >
          <DialogContent
            showCloseButton={false}
            onCloseAutoFocus={(event) => {
              event.preventDefault()
              heading.current?.focus()
            }}
          >
            <DialogHeader>
              <DialogTitle>{t('deleteProxy')}</DialogTitle>
              <DialogDescription>{deleting.name}</DialogDescription>
            </DialogHeader>
            <p className="text-sm leading-6">
              {t(
                deleting.account_count > 0
                  ? 'proxyInUse'
                  : 'deleteProxyConfirm',
              )}
            </p>
            {remove.isError && (
              <p role="alert" className="text-sm text-error">
                {t(proxyErrorKey(remove.error))}
              </p>
            )}
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => setDeleting(null)}
                disabled={remove.isPending}
              >
                {t('cancel')}
              </Button>
              <Button
                variant="destructive"
                disabled={deleting.account_count > 0 || remove.isPending}
                onClick={() => remove.mutate(deleting.id)}
              >
                {t('deleteProxy')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
      {pruning && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !prune.isPending) setPruning(false)
          }}
        >
          <DialogContent
            showCloseButton={false}
            onInteractOutside={(event) => {
              if (prune.isPending) event.preventDefault()
            }}
            onEscapeKeyDown={(event) => {
              if (prune.isPending) event.preventDefault()
            }}
            onCloseAutoFocus={(event) => {
              event.preventDefault()
              heading.current?.focus()
            }}
          >
            <DialogHeader>
              <DialogTitle>
                {t('deleteFailedProxies', { count: recentFailures })}
              </DialogTitle>
              <DialogDescription>{t('proxyPruneConfirm')}</DialogDescription>
            </DialogHeader>
            {prune.isError && (
              <p role="alert" className="text-sm text-error">
                {t('proxyPruneFailed')}
              </p>
            )}
            <DialogFooter>
              <Button
                variant="outline"
                disabled={prune.isPending}
                onClick={() => setPruning(false)}
              >
                {t('cancel')}
              </Button>
              <Button
                variant="destructive"
                disabled={recentFailures === 0 || prune.isPending}
                onClick={() => prune.mutate()}
              >
                {prune.isPending && (
                  <LoaderCircle
                    className="motion-safe:animate-spin"
                    aria-hidden="true"
                  />
                )}
                {t(
                  recentFailures === 1
                    ? 'proxyPruneAction_one'
                    : 'proxyPruneAction_other',
                  { count: recentFailures },
                )}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </div>
  )
}
