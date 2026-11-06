/** The window layout the user can shape: panel widths, which side the side panel sits on, file columns. */
export interface Layout {
  rackW: number
  sideW: number
  dock: 'left' | 'right'
  rackOpen: boolean
  sideOpen: boolean
  cols: { size: number; modified: number }
  xferH: number
}

export const DEFAULT: Layout = { rackW: 340, sideW: 380, dock: 'right', rackOpen: true, sideOpen: true, cols: { size: 68, modified: 96 }, xferH: 190 }

export const LIMITS = {
  rackW: [220, 560],
  sideW: [280, 760],
  size: [48, 160],
  modified: [72, 200],
  xferH: [90, 520],
} as const

export const clamp = (n: number, [lo, hi]: readonly [number, number]) => Math.min(hi, Math.max(lo, Math.round(n)))

const KEY = 'sshdeck.layout'

/** Whatever was stored is checked field by field, so an old or damaged value never breaks the window. */
export function sanitize(v: unknown): Layout {
  const o = (v && typeof v === 'object' ? v : {}) as Partial<Layout> & { cols?: Partial<Layout['cols']> }
  const num = (x: unknown, d: number, lim: readonly [number, number]) => (typeof x === 'number' && Number.isFinite(x) ? clamp(x, lim) : d)
  return {
    rackW: num(o.rackW, DEFAULT.rackW, LIMITS.rackW),
    sideW: num(o.sideW, DEFAULT.sideW, LIMITS.sideW),
    dock: o.dock === 'left' ? 'left' : 'right',
    rackOpen: o.rackOpen !== false,
    sideOpen: o.sideOpen !== false,
    cols: { size: num(o.cols?.size, DEFAULT.cols.size, LIMITS.size), modified: num(o.cols?.modified, DEFAULT.cols.modified, LIMITS.modified) },
    xferH: num(o.xferH, DEFAULT.xferH, LIMITS.xferH),
  }
}

export function loadLayout(): Layout {
  try {
    return sanitize(JSON.parse(localStorage.getItem(KEY) ?? 'null'))
  } catch {
    return DEFAULT
  }
}

export function saveLayout(l: Layout) {
  try {
    localStorage.setItem(KEY, JSON.stringify(l))
  } catch {
    /* the layout lasts for this run only */
  }
}
