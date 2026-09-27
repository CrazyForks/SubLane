import { createContext, useContext } from 'react'
import { z } from 'zod'
import { ApiError, request } from './request'

const schema = z.object({ time_zone: z.string().min(1).max(64) })
export const TimeZoneContext = createContext('UTC')

export function useTimeZone() {
  return useContext(TimeZoneContext)
}

export function validTimeZone(value: string) {
  if (
    !value ||
    value === 'Local' ||
    value.length > 64 ||
    value.trim() !== value
  )
    return false
  try {
    new Intl.DateTimeFormat('en', { timeZone: value })
    return true
  } catch {
    return false
  }
}

export function formatInstanceDate(
  timestamp: number,
  locale: string,
  timeZone: string,
  options: Intl.DateTimeFormatOptions,
) {
  return new Intl.DateTimeFormat(locale, { ...options, timeZone }).format(
    timestamp,
  )
}

export function saveTimeZone(timeZone: string, signal: AbortSignal) {
  return request('/api/settings/timezone', schema, {
    method: 'PATCH',
    body: JSON.stringify({ time_zone: timeZone }),
    signal,
  })
}

export function timeZoneErrorKey(error: Error) {
  if (error instanceof ApiError && error.code === 'invalid_time_zone')
    return 'timeZoneInvalid'
  return 'timeZoneSaveFailed'
}
