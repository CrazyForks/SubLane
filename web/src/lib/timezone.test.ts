import { expect, it, vi } from 'vitest'
import { formatInstanceDate, saveTimeZone, validTimeZone } from './timezone'

it('formats timestamps in the selected instance zone', () => {
  const timestamp = Date.UTC(2030, 2, 31, 16, 30)
  const options: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }
  expect(
    formatInstanceDate(timestamp, 'en-US', 'Asia/Shanghai', options),
  ).toContain('04/01/2030')
  expect(formatInstanceDate(timestamp, 'en-US', 'UTC', options)).toContain(
    '03/31/2030',
  )
})

it('accepts IANA zones and sends the selected zone to the instance setting', async () => {
  expect(validTimeZone('Asia/Shanghai')).toBe(true)
  expect(validTimeZone('Local')).toBe(false)
  expect(validTimeZone('Not/AZone')).toBe(false)
  const fetch = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ time_zone: 'Asia/Shanghai' }), {
      status: 200,
    }),
  )
  vi.stubGlobal('fetch', fetch)
  expect(
    await saveTimeZone('Asia/Shanghai', new AbortController().signal),
  ).toEqual({ time_zone: 'Asia/Shanghai' })
  expect(fetch.mock.calls[0][1].method).toBe('PATCH')
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({
    time_zone: 'Asia/Shanghai',
  })
})
