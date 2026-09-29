import { useEffect, useState, type RefObject } from 'react'
import { placeGuide, type Bounds } from '@/lib/spotlight'

type Spotlight = {
  target: Bounds | null
  paused: boolean
  width: number
  height: number
  left: number
  top: number
}

export function useSpotlight(
  selector: string | null,
  panel: RefObject<HTMLElement | null>,
  active: boolean,
) {
  const [value, setValue] = useState<Spotlight>({
    target: null,
    paused: false,
    width: 0,
    height: 0,
    left: 12,
    top: 12,
  })
  useEffect(() => {
    if (!active) return
    let frame: number | undefined
    let target: HTMLElement | null = null
    let observedPanel: HTMLElement | null = null
    const schedule = () => {
      if (frame !== undefined) return
      frame = window.requestAnimationFrame(read)
    }
    const resize =
      typeof ResizeObserver === 'undefined'
        ? null
        : new ResizeObserver(schedule)
    function read() {
      frame = undefined
      const paused = Boolean(
        document.querySelector(
          '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"], [role="menu"][data-state="open"]',
        ),
      )
      const next =
        selector && !paused
          ? document.querySelector<HTMLElement>(selector)
          : null
      if (next !== target) {
        if (target) resize?.unobserve(target)
        target = next
        if (target) {
          target.scrollIntoView?.({
            block: 'center',
            inline: 'nearest',
            behavior: 'instant',
          })
          resize?.observe(target)
        }
      }
      if (panel.current !== observedPanel) {
        if (observedPanel) resize?.unobserve(observedPanel)
        observedPanel = panel.current
        if (observedPanel) resize?.observe(observedPanel)
      }
      const box = target?.getBoundingClientRect()
      const rect =
        box && box.width > 0 && box.height > 0
          ? {
              left: box.left,
              top: box.top,
              width: box.width,
              height: box.height,
            }
          : null
      const viewport = { width: window.innerWidth, height: window.innerHeight }
      const size = {
        width: panel.current?.offsetWidth || 352,
        height: panel.current?.offsetHeight || 260,
      }
      const nextValue = {
        target: rect,
        paused,
        ...viewport,
        ...placeGuide(rect, viewport, size),
      }
      // The guide is itself observed. Publish geometry changes only, preventing a portal render loop.
      setValue((previous) =>
        JSON.stringify(previous) === JSON.stringify(nextValue)
          ? previous
          : nextValue,
      )
    }
    const mutations = new MutationObserver(schedule)
    mutations.observe(document.body, {
      subtree: true,
      childList: true,
      characterData: true,
      attributes: true,
      attributeFilter: ['data-state', 'aria-hidden', 'disabled'],
    })
    window.addEventListener('resize', schedule)
    window.addEventListener('scroll', schedule, true)
    schedule()
    return () => {
      if (frame !== undefined) window.cancelAnimationFrame(frame)
      resize?.disconnect()
      mutations.disconnect()
      window.removeEventListener('resize', schedule)
      window.removeEventListener('scroll', schedule, true)
    }
  }, [selector, panel, active])
  return value
}
