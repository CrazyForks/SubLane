import { useMutationState, useQuery } from '@tanstack/react-query'
import { FlaskConical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { authOptions } from '@/lib/auth'
import { ApiError } from '@/lib/request'

export function DemoNotice() {
  const { t } = useTranslation()
  const { data } = useQuery(authOptions())
  const restrictions = useMutationState({
    filters: { status: 'error' },
    select: (mutation) =>
      mutation.state.error instanceof ApiError &&
      mutation.state.error.code === 'demo_read_only',
  })
  if (!data?.demo) return null
  const blocked = restrictions.some(Boolean)
  return (
    <aside
      aria-label={t('demoTitle')}
      className="flex items-start gap-3 border-y border-border bg-muted/50 px-5 py-3 md:px-6"
    >
      <FlaskConical
        aria-hidden="true"
        className="mt-0.5 size-4 shrink-0 text-muted-foreground"
      />
      <div className="min-w-0 space-y-1 text-sm">
        <p className="font-medium">{t('demoTitle')}</p>
        <p
          role={blocked ? 'alert' : undefined}
          className="leading-6 text-muted-foreground"
        >
          {t(blocked ? 'demoReadOnly' : 'demoDescription')}
        </p>
      </div>
    </aside>
  )
}
