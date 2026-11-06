import { useEffect, useMemo, useRef, useState } from 'react'
import type { Backend, LogKind } from '../api'
import { levelOf, makeFilter, pushLines } from '../lib/logs'
import { messageOf } from '../lib/config'

interface Props {
  backend: Backend
  /** Server id, or "serverId@container" for a file inside a container. */
  target: string
  kind: LogKind
  arg: string
  visible: boolean
}

/** A followed log: newest lines at the bottom, a filter, pause and copy. Keeps the last 10,000 lines. */
export function LogView({ backend, target, kind, arg, visible }: Props) {
  const [lines, setLines] = useState<string[]>([])
  const [filter, setFilter] = useState('')
  const [paused, setPaused] = useState(false)
  const [wrap, setWrap] = useState(true)
  const [state, setState] = useState<'starting' | 'live' | 'ended'>('starting')
  const [why, setWhy] = useState('')
  const pausedRef = useRef(false)
  const held = useRef<string[]>([])
  const box = useRef<HTMLDivElement>(null)
  const stick = useRef(true) // follow the newest line until the user scrolls up
  pausedRef.current = paused

  useEffect(() => {
    let id = ''
    let off: Array<() => void> = []
    let gone = false
    const take = (l: string[]) => (pausedRef.current ? (held.current = pushLines(held.current, l)) : setLines((p) => pushLines(p, l)))
    backend.logs
      .start(target, kind, arg)
      .then((i) => {
        if (gone) return backend.logs.stop(i)
        id = i
        off = [backend.logs.onLines(i, take), backend.logs.onEnd(i, (w) => (setWhy(w), setState('ended')))]
        setState('live')
      })
      .catch((e) => (setWhy(messageOf(e)), setState('ended')))
    return () => {
      gone = true
      off.forEach((f) => f())
      if (id) backend.logs.stop(id)
    }
  }, [backend, target, kind, arg])

  const resume = () => {
    setLines((p) => pushLines(p, held.current))
    held.current = []
    setPaused(false)
  }
  const shown = useMemo(() => {
    const f = makeFilter(filter)
    return lines.filter(f)
  }, [lines, filter])

  useEffect(() => {
    const el = box.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [shown, visible])

  return (
    <div className="logview" hidden={!visible}>
      <div className="log-bar" role="toolbar" aria-label="Log controls">
        <input aria-label="Filter lines" placeholder="Filter (text, or /regex/)" value={filter} onChange={(e) => setFilter(e.target.value)} />
        <button className="btn" aria-pressed={paused} onClick={() => (paused ? resume() : setPaused(true))}>{paused ? 'Resume' : 'Pause'}</button>
        <button className="btn" aria-pressed={wrap} onClick={() => setWrap(!wrap)}>Wrap</button>
        <button className="btn" onClick={() => setLines([])}>Clear</button>
        <button className="btn" onClick={() => navigator.clipboard?.writeText(shown.join('\n')).catch(() => {})} disabled={!shown.length}>Copy</button>
        <span className="dim log-state">
          {state === 'starting' ? 'Starting…' : state === 'live' ? (paused ? 'Paused' : 'Following') : 'Stopped'}
          {filter.trim() ? ` · ${shown.length} of ${lines.length} lines` : ` · ${lines.length} lines`}
        </span>
      </div>
      <div
        className={`log-lines${wrap ? ' wrap' : ''}`}
        ref={box}
        tabIndex={0}
        role="log"
        aria-label={`Log: ${arg || kind}`}
        onScroll={(e) => {
          const el = e.currentTarget
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24
        }}
      >
        {shown.map((l, i) => (
          <div key={i} className={`log-line ${levelOf(l)}`}>{l || ' '}</div>
        ))}
        {state === 'ended' && <div className="log-line dim">{why ? `Stopped: ${why}` : 'The log stopped.'}</div>}
      </div>
    </div>
  )
}
