import { useCallback, useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useRouterState } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight, CircleCheck, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { authKey, type AuthState } from '@/lib/auth'
import { connectionOptions } from '@/lib/connection'
import { setupOrder, setupSteps } from '@/lib/activation'
import { useTour } from '@/lib/tour'
import { useSpotlight } from '@/hooks/use-spotlight'
import { Button } from './ui/Button'

export function FloatingTour() {
  const tour = useTour()!
  const { t } = useTranslation()
  const client = useQueryClient()
  const path = useRouterState({ select: (state) => state.location.pathname })
  const query = useQuery({
    ...connectionOptions(client, tour.userID),
    enabled: () =>
      Boolean(tour.stage) &&
      client.getQueryData<AuthState>(authKey)?.user?.id === tour.userID,
    refetchInterval: tour.stage ? 3000 : false,
  })
  const order = setupOrder(tour.administrator)
  const setup = query.data?.setup
  const frontier =
    setup?.stage === 'complete'
      ? order.length
      : Math.max(
          0,
          order.findIndex((step) => step === setup?.stage),
        )
  const selected = Math.max(
    0,
    order.findIndex((step) => step === tour.stage),
  )
  const index = Math.min(selected, frontier, order.length - 1)
  // Completed workspaces can replay earlier steps without changing their saved completion state.
  const finished =
    setup?.stage === 'complete' && index === order.length - 1 && !query.isError
  const step = order[index]
  const metadata = setupSteps[step]
  const onPage = path.replace(/\/+$/, '') === metadata.to
  const selector = onPage ? `[data-tour="${step}"]:not(:disabled)` : null
  const panel = useRef<HTMLElement>(null)
  const title = useRef<HTMLHeadingElement>(null)
  const spotlight = useSpotlight(selector, panel, Boolean(tour.stage))
  const ready = Boolean(setup) && !query.isError && !query.isPending
  const canNext = ready && index < frontier
  const stop = useCallback(() => {
    const target = selector
      ? document.querySelector<HTMLElement>(selector)
      : null
    tour.stop()
    target?.focus({ preventScroll: true })
  }, [selector, tour])
  const visible = spotlight.width > 0
  useEffect(() => {
    if (!tour.stage || spotlight.paused || !visible) return
    title.current?.focus({ preventScroll: true })
  }, [tour.stage, path, spotlight.paused, visible])
  useEffect(() => {
    if (!tour.stage || spotlight.paused) return
    const dismiss = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        stop()
      }
    }
    document.addEventListener('keydown', dismiss)
    return () => document.removeEventListener('keydown', dismiss)
  }, [tour.stage, stop, spotlight.paused])
  if (!tour.stage || spotlight.paused) return null
  const target = spotlight.target
  return createPortal(
    <>
      {target && (
        <div
          aria-hidden="true"
          data-tour-highlight
          className="pointer-events-none fixed z-40 rounded-lg border-2 border-primary"
          style={{
            left: Math.max(2, target.left - 6),
            top: Math.max(2, target.top - 6),
            width: target.width + 12,
            height: target.height + 12,
            boxShadow: '0 0 0 9999px rgb(0 0 0 / 0.38)',
          }}
        />
      )}
      <aside
        ref={panel}
        role="region"
        aria-label={t('tourTitle')}
        className="fixed z-[45] w-[min(22rem,calc(100vw-1.5rem))] max-h-[calc(100dvh-1.5rem)] overflow-y-auto rounded-xl border border-border bg-popover p-5 text-popover-foreground shadow-lg"
        style={{
          left: spotlight.left,
          top: spotlight.top,
          visibility: spotlight.width ? 'visible' : 'hidden',
        }}
      >
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground" aria-live="polite">
            {t('tourStep', {
              current: finished ? order.length : index + 1,
              total: order.length,
            })}
          </p>
          <Button
            variant="ghost"
            size="icon"
            className="size-9 shrink-0"
            onClick={stop}
            aria-label={t('tourSkip')}
          >
            <X className="size-4" aria-hidden="true" />
          </Button>
        </div>
        <h2
          ref={title}
          tabIndex={-1}
          className="mt-2 text-base font-semibold outline-none"
        >
          {t(finished ? 'activationComplete' : metadata.title)}
        </h2>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {t(finished ? 'activationCompleteHint' : metadata.hint)}
        </p>
        {query.isError ||
        tour.navigationFailed ||
        (!query.isPending && !setup) ? (
          <p role="alert" className="mt-3 text-sm text-error">
            {t('tourUnavailable')}
          </p>
        ) : canNext && !finished ? (
          <p
            role="status"
            className="mt-3 flex items-center gap-2 text-sm text-success"
          >
            <CircleCheck className="size-4 shrink-0" aria-hidden="true" />
            {t('tourStepDone')}
          </p>
        ) : (
          !finished && (
            <p
              role="status"
              className="mt-3 text-xs leading-5 text-muted-foreground"
            >
              {t(
                query.isPending
                  ? 'gatewayChecking'
                  : step === 'access'
                    ? 'tourAccessWaiting'
                    : 'tourWaiting',
              )}
            </p>
          )
        )}
        {!onPage && !finished && (
          <Button
            className="mt-4 w-full"
            variant="outline"
            onClick={() => tour.move(step)}
          >
            {t('tourOpenStep')}
          </Button>
        )}
        <div className="mt-5 flex flex-wrap items-center justify-between gap-2 border-t border-border pt-4">
          <Button
            variant="ghost"
            size="sm"
            disabled={query.isFetching}
            onClick={() => query.refetch()}
          >
            {t('tourCheck')}
          </Button>
          <div className="flex gap-2">
            {!finished && (
              <Button
                variant="outline"
                size="icon"
                disabled={index === 0}
                onClick={() => tour.move(order[index - 1])}
                aria-label={t('tourPrevious')}
              >
                <ArrowLeft className="size-4" aria-hidden="true" />
              </Button>
            )}
            {finished ? (
              <Button size="sm" onClick={stop}>
                {t('tourFinish')}
              </Button>
            ) : (
              <Button
                size="sm"
                disabled={!canNext}
                onClick={() => tour.move(order[index + 1])}
              >
                {t('tourNext')}
                <ArrowRight className="size-4" aria-hidden="true" />
              </Button>
            )}
          </div>
        </div>
      </aside>
    </>,
    document.body,
  )
}
