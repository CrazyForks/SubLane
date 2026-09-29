import { useState, type ReactNode } from 'react'
import { useRouter } from '@tanstack/react-router'
import { CircleHelp } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { setupOrder, setupSteps, type SetupProgress } from '@/lib/activation'
import { TourContext, useTour } from '@/lib/tour'
import { FloatingTour } from './FloatingTour'
import { Button } from './ui/Button'

export function TourButton() {
  const { t } = useTranslation()
  const tour = useTour()
  if (!tour) return null
  return (
    <Button
      variant="ghost"
      className="text-muted-foreground [@media(pointer:coarse)]:min-h-11"
      title={t('tourOpen')}
      aria-label={t('tourOpen')}
      onClick={() => tour.move(setupOrder(tour.administrator)[0])}
    >
      <CircleHelp aria-hidden="true" />
      <span className="hidden sm:inline">{t('tourOpen')}</span>
    </Button>
  )
}

export function TourProvider({
  userID,
  administrator,
  workspace,
  children,
}: {
  userID: number
  administrator: boolean
  workspace: number
  children: ReactNode
}) {
  const router = useRouter()
  const storageKey = `sublane.tour.${userID}.${workspace}.${administrator ? 'admin' : 'member'}`
  const [stage, setStage] = useState<SetupProgress['stage'] | null>(() => {
    try {
      const saved = sessionStorage.getItem(storageKey)
      return setupOrder(administrator).find((step) => step === saved) ?? null
    } catch {
      return null
    }
  })
  const [navigationFailed, setNavigationFailed] = useState(false)
  const stop = () => {
    setStage(null)
    setNavigationFailed(false)
    try {
      sessionStorage.removeItem(storageKey)
    } catch {
      /* The guide still works without browser storage. */
    }
  }
  const move = async (next: SetupProgress['stage']) => {
    if (!setupOrder(administrator).some((step) => step === next)) return
    setStage(next)
    setNavigationFailed(false)
    try {
      sessionStorage.setItem(storageKey, next)
    } catch {
      /* Progress is still derived from server state. */
    }
    try {
      await router.navigate({ to: setupSteps[next].to })
    } catch {
      setNavigationFailed(true)
    }
  }
  return (
    <TourContext.Provider
      value={{ stage, userID, administrator, navigationFailed, move, stop }}
    >
      {children}
      {stage && <FloatingTour />}
    </TourContext.Provider>
  )
}
