import { useRef, useState } from 'react'

interface Props {
  /** Current size of the panel this handle resizes, in pixels. */
  value: number
  onChange: (px: number) => void
  /** +1 when growing means dragging right (a panel on the left of the handle), -1 for a panel on its right. */
  dir: 1 | -1
  label: string
  /** Double-click puts the panel back to this size. */
  reset: number
  vertical?: boolean
  className?: string
}

/** A drag handle between two panels: pointer drag, arrow keys (Shift for bigger steps), double-click to reset. */
export function Splitter({ value, onChange, dir, label, reset, vertical, className }: Props) {
  const start = useRef<{ at: number; v: number } | null>(null)
  const [drag, setDrag] = useState(false)
  const pos = (e: { clientX: number; clientY: number }) => (vertical ? e.clientY : e.clientX)
  return (
    <div
      className={`split${className ? ' ' + className : ''}`}
      data-vertical={vertical || undefined}
      data-drag={drag || undefined}
      role="separator"
      aria-orientation={vertical ? 'horizontal' : 'vertical'}
      aria-label={label}
      aria-valuenow={value}
      tabIndex={0}
      title={`${label} (drag, arrow keys, double-click to reset)`}
      onPointerDown={(e) => {
        e.currentTarget.setPointerCapture(e.pointerId)
        start.current = { at: pos(e), v: value }
        setDrag(true)
      }}
      onPointerMove={(e) => start.current && onChange(start.current.v + dir * (pos(e) - start.current.at))}
      onPointerUp={() => ((start.current = null), setDrag(false))}
      onPointerCancel={() => ((start.current = null), setDrag(false))}
      onDoubleClick={() => onChange(reset)}
      onKeyDown={(e) => {
        const step = e.shiftKey ? 64 : 16
        const grow = vertical ? 'ArrowUp' : dir > 0 ? 'ArrowRight' : 'ArrowLeft'
        const shrink = vertical ? 'ArrowDown' : dir > 0 ? 'ArrowLeft' : 'ArrowRight'
        if (e.key === grow) onChange(value + step)
        else if (e.key === shrink) onChange(value - step)
        else if (e.key === 'Home') onChange(0)
        else if (e.key === 'End') onChange(99999)
        else return
        e.preventDefault()
      }}
    />
  )
}
