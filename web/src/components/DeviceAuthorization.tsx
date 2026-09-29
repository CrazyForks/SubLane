import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { accountErrorKey, pollAuthorization } from '@/lib/accounts'
import { Button } from './ui/Button'

type Props = {
  authorization: {
    state: string
    user_code?: string
    expires_at: number
    interval?: number
  }
  onConnected: () => Promise<void>
  onRestart: () => void
}

export function DeviceAuthorization({
  authorization,
  onConnected,
  onRestart,
}: Props) {
  const { t } = useTranslation()
  const [now, setNow] = useState(() => Date.now())
  const [completionError, setCompletionError] = useState<unknown>(null)
  const delivered = useRef(false)
  const expired = now >= authorization.expires_at * 1000
  const poll = useQuery({
    queryKey: ['device-authorization', authorization.state],
    queryFn: ({ signal }) => pollAuthorization(authorization.state, signal),
    gcTime: 0,
    retry: false,
    enabled: !expired,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    refetchInterval: (query) =>
      query.state.data?.account || query.state.error || expired
        ? false
        : Math.max(
            5,
            query.state.data?.interval ?? authorization.interval ?? 5,
          ) * 1000,
  })
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  useEffect(() => {
    if (poll.data?.account && !delivered.current) {
      delivered.current = true
      onConnected().catch(setCompletionError)
    }
  }, [poll.data?.account, onConnected])
  const failure = poll.error ?? completionError
  return (
    <div className="space-y-3">
      <p className="text-sm font-medium">{t('deviceCode')}</p>
      <p className="select-all break-all font-mono text-xl tracking-wider">
        {authorization.user_code}
      </p>
      {failure || expired ? (
        <>
          <p role="alert" className="text-sm leading-6 text-error">
            {t(failure ? accountErrorKey(failure) : 'oauthStateInvalid')}
          </p>
          <Button type="button" variant="outline" onClick={onRestart}>
            {t('startAgain')}
          </Button>
        </>
      ) : (
        <p role="status" className="text-sm leading-6 text-muted-foreground">
          {t('deviceAuthorizationWaiting')}
        </p>
      )}
    </div>
  )
}
