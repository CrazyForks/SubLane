import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { allocationOptions, modeLabels } from '@/lib/allocations'
import { Button } from './ui/Button'
import { AllocationBalances } from './AllocationBalances'
import { AllocationSettlement } from './AllocationSettlement'

export function AllocationReport({ id }: { id: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(allocationOptions(id))
  const reload = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['allocation', id] }),
      client.invalidateQueries({ queryKey: ['own-allocations'] }),
    ])
  }
  if (query.isPending) return <p role="status">{t('allocationLoading')}</p>
  if (query.isError)
    return (
      <div role="alert">
        <p>{t('allocationFailed')}</p>
        <Button onClick={() => query.refetch()}>{t('reconnect')}</Button>
      </div>
    )
  const d = query.data
  return (
    <section className="max-w-4xl space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="break-words text-lg font-semibold">{d.name}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {d.group_name} · {t(modeLabels[d.config.mode])}
          </p>
        </div>
        <Button
          variant="outline"
          disabled={query.isFetching}
          onClick={() => query.refetch()}
        >
          {t('refresh')}
        </Button>
      </div>
      <AllocationBalances detail={d} />
      {d.pending.length > 0 && (
        <section className="space-y-4 border-t border-border pt-5">
          <h3 className="font-medium">{t('allocationPending')}</h3>
          <p className="text-sm leading-6 text-muted-foreground">
            {t('allocationSettlementHint')}
          </p>
          {d.pending.map((p) => (
            <AllocationSettlement
              key={p.request_id}
              id={id}
              entry={p}
              onSaved={reload}
            />
          ))}
        </section>
      )}
    </section>
  )
}
