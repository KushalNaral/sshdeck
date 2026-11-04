import { useEffect, useMemo, useRef, useState } from 'react'
import { markFor, viaLabel } from '../lib/status'
import type { Server, Session, Vpn } from '../types'
import { Mark } from './Mark'

interface Props {
  servers: Server[]
  vpns: Vpn[]
  sessions: Record<string, Session>
  onPick: (id: string, connect: boolean) => void
  onClose: () => void
}

/** Ctrl+K: type a server or a project name. Enter connects, Shift+Enter only selects. */
export function Palette({ servers, vpns, sessions, onPick, onClose }: Props) {
  const [q, setQ] = useState('')
  const [i, setI] = useState(0)
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => input.current?.focus(), [])

  const hits = useMemo(() => {
    const n = q.trim().toLowerCase()
    return servers
      .map((s) => ({ s, via: s.projects.find((p) => p.toLowerCase().includes(n)) }))
      .filter(({ s, via }) => !n || s.name.toLowerCase().includes(n) || via)
      .slice(0, 12)
  }, [q, servers])

  const key = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') onClose()
    else if (e.key === 'ArrowDown') (e.preventDefault(), setI((v) => Math.min(v + 1, hits.length - 1)))
    else if (e.key === 'ArrowUp') (e.preventDefault(), setI((v) => Math.max(v - 1, 0)))
    else if (e.key === 'Enter' && hits[i]) onPick(hits[i].s.id, !e.shiftKey)
  }

  return (
    <div className="overlay" onMouseDown={onClose}>
      <div className="palette" role="dialog" aria-label="Find a server or project" onMouseDown={(e) => e.stopPropagation()} onKeyDown={key}>
        <input
          ref={input}
          value={q}
          placeholder="Server or project name"
          aria-label="Server or project name"
          onChange={(e) => {
            setQ(e.target.value)
            setI(0)
          }}
        />
        <ul role="listbox">
          {hits.map(({ s, via }, idx) => (
            <li key={s.id} role="option" aria-selected={idx === i} onMouseEnter={() => setI(idx)} onClick={() => onPick(s.id, true)}>
              <Mark kind={markFor(s, sessions[s.id])} />
              <strong className="name">{s.name}</strong>
              <span style={{ marginLeft: 'auto', opacity: 0.8 }}>{via ?? viaLabel(s, vpns)}</span>
            </li>
          ))}
          {hits.length === 0 && <li style={{ cursor: 'default' }}>No server or project matches "{q}".</li>}
        </ul>
      </div>
    </div>
  )
}
