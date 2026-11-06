import { useEffect, useRef, useState } from 'react'
import { niceMax, withGaps, type Pt } from '../lib/metrics'

export interface Line {
  name: string
  pts: Pt[]
  /** 1 is blue, 2 is orange (the first two slots of the chart palette). */
  slot: 1 | 2
}

interface Props {
  title: string
  lines: Line[]
  fmt: (v: number) => string
  /** The y-axis never tops out under this, so a quiet server does not look busy. */
  floor: number
  /** A fixed top, for percentages. */
  max?: number
  /** A reference line, such as the core count for load. */
  refLine?: { v: number; label: string }
  height?: number
}

const M = { l: 66, r: 14, t: 8, b: 20 }

const clock = (t: number, span: number) => {
  const d = new Date(t * 1000)
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return span > 86400 ? `${d.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${time}` : time
}

function useWidth() {
  const el = useRef<HTMLDivElement>(null)
  const [w, setW] = useState(560)
  useEffect(() => {
    if (!el.current) return
    const ro = new ResizeObserver(([e]) => setW(Math.max(240, Math.round(e.contentRect.width))))
    ro.observe(el.current)
    return () => ro.disconnect()
  }, [])
  return [el, w] as const
}

/** Breaks a series into runs of real values so a hole is a gap in the line. */
function runs(pts: Pt[]): Pt[][] {
  const out: Pt[][] = []
  let cur: Pt[] = []
  for (const p of withGaps(pts)) {
    if (p.v === null) {
      if (cur.length) out.push(cur)
      cur = []
    } else cur.push(p)
  }
  if (cur.length) out.push(cur)
  return out
}

export function LineChart({ title, lines, fmt, floor, max, refLine, height = 150 }: Props) {
  const [box, w] = useWidth()
  const [hover, setHover] = useState<number | null>(null)
  const all = lines.flatMap((l) => l.pts)
  const ts = all.map((p) => p.t)
  const t0 = Math.min(...ts), t1 = Math.max(...ts)
  const span = t1 - t0
  const top = max ?? niceMax([...all.map((p) => p.v), refLine?.v ?? 0], floor)
  const iw = w - M.l - M.r, ih = height - M.t - M.b
  const X = (t: number) => M.l + (span ? ((t - t0) / span) * iw : iw)
  const Y = (v: number) => M.t + ih - (Math.min(v, top) / top) * ih
  const empty = !isFinite(t0) || all.every((p) => p.v === null)

  const move = (e: React.MouseEvent<SVGSVGElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    const t = t0 + ((e.clientX - r.left - M.l) / iw) * span
    const base = lines[0].pts
    let best = 0
    for (let i = 1; i < base.length; i++) if (Math.abs(base[i].t - t) < Math.abs(base[best].t - t)) best = i
    setHover(base[best]?.t ?? null)
  }
  // The nearest reading, not an exact match: the data is refreshed while the pointer rests on the chart.
  const at = (l: Line, t: number) => {
    let best: Pt | undefined
    for (const p of l.pts) if (!best || Math.abs(p.t - t) < Math.abs(best.t - t)) best = p
    return best?.v ?? null
  }
  const ticks = [0, top / 2, top]
  const rows = lines[0]?.pts.filter((_, i, a) => i % Math.max(1, Math.ceil(a.length / 24)) === 0) ?? []

  return (
    <figure className="viz" ref={box} style={{ margin: 0 }}>
      <figcaption className="viz-head">
        <span className="viz-title">{title}</span>
        {lines.length > 1 && (
          <span className="viz-legend">
            {lines.map((l) => (
              <span key={l.name}>
                <i className={`sw s${l.slot}`} /> {l.name}
              </span>
            ))}
          </span>
        )}
      </figcaption>
      {empty ? (
        <div className="viz-empty" style={{ height }}>
          No readings in this range.
        </div>
      ) : (
        <div style={{ position: 'relative' }}>
          <svg width={w} height={height} role="img" aria-label={`${title}, ${lines.map((l) => l.name).join(' and ')}`} onMouseMove={move} onMouseLeave={() => setHover(null)}>
            {ticks.map((v) => (
              <g key={v}>
                <line x1={M.l} x2={w - M.r} y1={Y(v)} y2={Y(v)} className="viz-grid" />
                <text x={M.l - 6} y={Y(v) + 4} textAnchor="end" className="viz-axis">
                  {fmt(v)}
                </text>
              </g>
            ))}
            {[t0, (t0 + t1) / 2, t1].map((t, i) => (
              <text key={i} x={X(t)} y={height - 5} textAnchor={i === 0 ? 'start' : i === 2 ? 'end' : 'middle'} className="viz-axis">
                {clock(t, span)}
              </text>
            ))}
            {refLine && (
              <g>
                <line x1={M.l} x2={w - M.r} y1={Y(refLine.v)} y2={Y(refLine.v)} className="viz-ref" />
                <text x={w - M.r - 2} y={Y(refLine.v) + 12} textAnchor="end" className="viz-axis">
                  {refLine.label}
                </text>
              </g>
            )}
            {lines.map((l) =>
              runs(l.pts).map((run, i) => {
                const d = run.map((p, j) => `${j ? 'L' : 'M'}${X(p.t).toFixed(1)},${Y(p.v as number).toFixed(1)}`).join('')
                return (
                  <g key={l.name + i}>
                    {lines.length === 1 && run.length > 1 && <path d={`${d}L${X(run[run.length - 1].t)},${Y(0)}L${X(run[0].t)},${Y(0)}Z`} className={`viz-area s${l.slot}`} />}
                    <path d={d} className={`viz-line s${l.slot}`} />
                  </g>
                )
              }),
            )}
            {hover !== null && (
              <g>
                <line x1={X(hover)} x2={X(hover)} y1={M.t} y2={M.t + ih} className="viz-cross" />
                {lines.map((l) => {
                  const v = at(l, hover)
                  return v === null ? null : <circle key={l.name} cx={X(hover)} cy={Y(v)} r={4} className={`viz-dot s${l.slot}`} />
                })}
              </g>
            )}
          </svg>
          {hover !== null && (
            <div className="viz-tip" style={{ left: Math.min(w - 150, Math.max(M.l, X(hover) + 10)), top: M.t }}>
              <b>{clock(hover, span)}</b>
              {lines.map((l) => (
                <div key={l.name}>
                  <i className={`sw s${l.slot}`} /> {l.name}: {at(l, hover) === null ? 'no reading' : fmt(at(l, hover) as number)}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
      {!empty && (
        <details className="viz-table">
          <summary>Table</summary>
          <table>
            <thead>
              <tr>
                <th>Time</th>
                {lines.map((l) => (
                  <th key={l.name}>{l.name}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((p) => (
                <tr key={p.t}>
                  <td>{clock(p.t, span)}</td>
                  {lines.map((l) => (
                    <td key={l.name}>{at(l, p.t) === null ? '' : fmt(at(l, p.t) as number)}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      )}
    </figure>
  )
}

/** A tiny trend line with no axes, for the fleet cards. The last value gets a dot. */
export function Spark({ pts, max = 100, slot = 1, label }: { pts: Pt[]; max?: number; slot?: 1 | 2; label: string }) {
  const W = 110, H = 28
  const good = pts.filter((p) => p.v !== null)
  if (good.length < 2) return <svg width={W} height={H} role="img" aria-label={`${label}: not enough readings yet`} />
  const t0 = good[0].t, t1 = good[good.length - 1].t
  const X = (t: number) => 3 + ((t - t0) / Math.max(1, t1 - t0)) * (W - 6)
  const Y = (v: number) => H - 3 - (Math.min(v, max) / max) * (H - 6)
  const last = good[good.length - 1]
  return (
    <svg width={W} height={H} role="img" aria-label={`${label}: now ${Math.round(last.v as number)}`}>
      {runs(pts).map((run, i) => (
        <path key={i} d={run.map((p, j) => `${j ? 'L' : 'M'}${X(p.t).toFixed(1)},${Y(p.v as number).toFixed(1)}`).join('')} className={`viz-line s${slot}`} />
      ))}
      <circle cx={X(last.t)} cy={Y(last.v as number)} r={3.5} className={`viz-dot s${slot}`} />
    </svg>
  )
}
