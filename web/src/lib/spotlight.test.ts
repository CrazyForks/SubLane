import { expect, it } from 'vitest'
import { placeGuide } from './spotlight'

it('keeps a guide near its target and inside a narrow viewport', () => {
  const position = placeGuide(
    { left: 20, top: 660, width: 120, height: 36 },
    { width: 390, height: 844 },
    { width: 352, height: 260 },
  )
  expect(position.left).toBeGreaterThanOrEqual(12)
  expect(position.left + 352).toBeLessThanOrEqual(378)
  expect(position.top + 260).toBeLessThanOrEqual(648)
})

it('places a guide below a top-right target without horizontal overflow', () => {
  const position = placeGuide(
    { left: 880, top: 100, width: 140, height: 40 },
    { width: 1024, height: 768 },
    { width: 352, height: 260 },
  )
  expect(position.left + 352).toBeLessThanOrEqual(1012)
  expect(position.top).toBeGreaterThanOrEqual(152)
})

it('keeps the fallback panel visible when no target is available', () => {
  expect(
    placeGuide(null, { width: 390, height: 400 }, { width: 352, height: 600 }),
  ).toEqual({ left: 26, top: 12 })
})
