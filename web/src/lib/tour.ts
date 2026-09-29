import { createContext, useContext } from 'react'
import type { SetupProgress } from './activation'

export type TourState = {
  stage: SetupProgress['stage'] | null
  userID: number
  administrator: boolean
  navigationFailed: boolean
  move: (stage: SetupProgress['stage']) => Promise<void>
  stop: () => void
}
export const TourContext = createContext<TourState | null>(null)
export const useTour = () => useContext(TourContext)
