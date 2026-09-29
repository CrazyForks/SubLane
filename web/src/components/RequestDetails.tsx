import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { authOptions } from '@/lib/auth'
import { requestRecovery } from '@/lib/recovery'
import { Button } from './ui/Button'
import type { RequestRecord } from '@/lib/requests'
import { outcomeKeys, formatRequestCost } from '@/lib/requests'
import { reasonKeys } from '@/lib/runtime'
import { RequestID } from './RequestID'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from './ui/Dialog'

export function RequestDetails({
  value,
  onClose,
  onRestoreFocus,
}: {
  value: RequestRecord | null
  onClose: () => void
  onRestoreFocus: () => void
}) {
  const { t, i18n } = useTranslation()
  const { data: auth } = useQuery(authOptions())
  if (!value) return null
  const recovery = requestRecovery(
    value.error_code,
    auth?.user?.role === 'admin',
  )
  const numbers = new Intl.NumberFormat(i18n.resolvedLanguage ?? 'en')
  const ms = (n: number | null) =>
    n === null ? '—' : `${numbers.format(n)} ms`
  const rows = [
    [t('requestModel'), value.model || '—'],
    [t('requestReasoningEffort'), value.reasoning_effort || '—'],
    [
      t('requestCaller'),
      `${value.username || `#${value.user_id}`} · ${value.key_name || `#${value.key_id}`}`,
    ],
    [t('requestGroup'), value.group_name || `#${value.group_id}`],
    ...(value.account_name ? [[t('requestAccount'), value.account_name]] : []),
    [
      t('requestTransport'),
      `${value.provider} · ${value.transport === 'websocket' ? 'WebSocket' : 'HTTP'} · ${value.operation}`,
    ],
    [t('requestResult'), t(outcomeKeys[value.outcome])],
    ...(value.error_code
      ? [
          [
            t('requestReason'),
            t(reasonKeys[value.error_code] ?? 'reasonUnknown'),
          ],
        ]
      : []),
    [t('requestUpstreamStatus'), value.upstream_status?.toString() ?? '—'],
    [t('requestDuration'), ms(value.duration_ms)],
    [t('requestFirstToken'), ms(value.first_token_ms)],
    [
      t('requestEstimatedCost'),
      formatRequestCost(
        value.estimated_cost_micro_usd,
        i18n.resolvedLanguage ?? 'en',
      ),
    ],
  ]
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent
        className="max-h-[85svh] overflow-y-auto"
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          onRestoreFocus()
        }}
      >
        <DialogHeader>
          <DialogTitle>{t('requestDetails')}</DialogTitle>
          <DialogDescription>{t('requestDetailsHint')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">{t('requestID')}</p>
          <RequestID value={value.request_id} />
        </div>
        <dl className="divide-y divide-border">
          {rows.map(([label, content]) => (
            <div
              key={label}
              className="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-4 py-2.5 text-sm"
            >
              <dt className="text-muted-foreground">{label}</dt>
              <dd className="break-words text-right tabular-nums">{content}</dd>
            </div>
          ))}
        </dl>
        {['error', 'rejected', 'incomplete'].includes(value.outcome) && (
          <section
            className="space-y-3 border-t border-border pt-4"
            aria-label={t('recoveryTitle')}
          >
            <h3 className="text-sm font-medium">{t('recoveryTitle')}</h3>
            <p className="text-sm leading-6 text-muted-foreground">
              {t(recovery.hint)}
            </p>
            {recovery.to && recovery.action && (
              <Button asChild variant="outline">
                <Link to={recovery.to} onClick={onClose}>
                  {t(recovery.action)}
                </Link>
              </Button>
            )}
          </section>
        )}
        <p className="text-xs leading-5 text-muted-foreground">
          {t('requestFirstTokenHint')}
        </p>
        <p className="text-xs leading-5 text-muted-foreground">
          {t('requestEstimatedCostHint')}
        </p>
      </DialogContent>
    </Dialog>
  )
}
