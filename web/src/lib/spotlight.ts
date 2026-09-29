export type Bounds = {
  left: number
  top: number
  width: number
  height: number
}
export type Size = { width: number; height: number }

export function placeGuide(target: Bounds | null, viewport: Size, panel: Size) {
  const gap = 12
  const width = Math.min(panel.width, viewport.width - gap * 2)
  const height = Math.min(panel.height, viewport.height - gap * 2)
  const right = Math.max(gap, viewport.width - gap - width)
  const bottom = Math.max(gap, viewport.height - gap - height)
  if (!target) return { left: right, top: bottom }
  const left = Math.max(gap, Math.min(target.left, right))
  const below = target.top + target.height + gap
  if (below + height <= viewport.height - gap) return { left, top: below }
  const above = target.top - gap - height
  if (above >= gap) return { left, top: above }
  const beside = target.left + target.width + gap
  if (beside + width <= viewport.width - gap)
    return { left: beside, top: Math.max(gap, Math.min(target.top, bottom)) }
  if (target.left - gap - width >= gap)
    return {
      left: target.left - gap - width,
      top: Math.max(gap, Math.min(target.top, bottom)),
    }
  return { left, top: bottom }
}
