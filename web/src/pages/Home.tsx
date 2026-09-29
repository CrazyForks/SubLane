import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { authOptions } from '@/lib/auth'
import { getSystem } from '@/lib/api'
import { Status } from '@/components/Status'
import { Button } from '@/components/ui/Button'
import { Usage } from './Usage'
import { Activation, PersonalActivation } from '@/components/Activation'

export function Home() {
  const { data } = useQuery(authOptions())
  const { t } = useTranslation()
  const user = data?.user
  if (!user) return null
  return (
    <div className="space-y-10">
      {user.role === 'admin' ? (
        <AdminHome />
      ) : (
        <div className="max-w-3xl space-y-8">
          <div>
            <h1 className="page-title">{t('memberOverviewTitle')}</h1>
          </div>
          <section className="rounded-xl border border-border bg-card p-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h2 className="font-semibold">{t('yourAccount')}</h2>
              <Status kind="success">{t('active')}</Status>
            </div>
            <dl className="mt-6 grid gap-5 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-muted-foreground">{t('username')}</dt>
                <dd className="mt-2 font-medium">{user.username}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">{t('role')}</dt>
                <dd className="mt-2 font-medium">{t('member')}</dd>
              </div>
            </dl>
          </section>
          <PersonalActivation userID={user.id} />
        </div>
      )}
      <Usage />
    </div>
  )
}

function AdminHome() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['system'],
    queryFn: ({ signal }) => getSystem(signal),
  })
  const gateway = query.data?.gateway
  const ready = gateway?.status === 'ready' && gateway.has_usable_key
  return (
    <div className="space-y-6">
      <div>
        <h1 className="page-title">{t('overviewTitle')}</h1>
      </div>
      {query.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t('loading')}
        </p>
      ) : query.isError ? (
        <div
          role="alert"
          className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground"
        >
          <span>{t('connectionError')}</span>
          <Button
            variant="outline"
            onClick={() => query.refetch()}
            disabled={query.isFetching}
          >
            {t('reconnect')}
          </Button>
        </div>
      ) : gateway?.setup && gateway.setup.stage !== 'complete' ? (
        <Activation setup={gateway.setup} administrator />
      ) : gateway && !ready ? (
        <section
          aria-labelledby="gateway-prompt-title"
          className="flex flex-wrap items-center gap-x-5 gap-y-3 rounded-xl border border-border bg-card px-5 py-4"
        >
          <div className="min-w-0 flex-1">
            <h2 id="gateway-prompt-title" className="font-semibold">
              {t('gatewaySetup')}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {t(
                gateway.status === 'needs_attention'
                  ? 'gatewayAttentionDescription'
                  : gateway.status === 'ready'
                    ? 'gatewayKeyNeededDescription'
                    : 'gatewaySetupDescription',
              )}
            </p>
          </div>
          <Button asChild variant="outline">
            <Link to={gateway.status === 'ready' ? '/keys' : '/accounts'}>
              {t(
                gateway.status === 'ready' ? 'createKeyTitle' : 'viewAccounts',
              )}
            </Link>
          </Button>
        </section>
      ) : null}
    </div>
  )
}
