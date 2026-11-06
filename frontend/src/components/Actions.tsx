import { useState } from 'react'
import type { Backend, ServiceAct } from '../api'
import { messageOf } from '../lib/config'
import type { Container, Server } from '../types'
import { Mark } from './Mark'

export type Ask = (a: { title: string; body: string; label: string; run: () => void }) => void

interface Shared {
  server: Server
  connected: boolean
  backend: Backend
  ask: Ask
  /** Called after an action that changed something, so the page can read the server again. */
  onDone: () => void
}

/** Runs one action: asks first on a production server and for any stop, shows what the server said. */
function useAct({ server, connected, ask, onDone }: Shared) {
  const [busy, setBusy] = useState(false)
  const [out, setOut] = useState<{ text: string; bad: boolean } | null>(null)
  const act = (verb: string, what: string, call: () => Promise<{ output: string; exit: number; sudo?: boolean }>, changes: boolean) => {
    const go = async () => {
      setBusy(true)
      setOut(null)
      try {
        const r = await call()
        const text = (r.output || (r.exit === 0 ? 'Done.' : `Exit code ${r.exit}.`)) + (r.sudo ? '\n(run with sudo)' : '')
        setOut({ text, bad: changes && r.exit !== 0 })
        if (changes) onDone()
      } catch (e) {
        setOut({ text: messageOf(e), bad: true })
      }
      setBusy(false)
    }
    if (!changes || (!server.production && verb !== 'Stop')) return void go()
    ask({ title: `${verb} ${what}?`, body: `${verb} ${what} on ${server.name}${server.production ? ' (production)' : ''}.`, label: verb, run: () => void go() })
  }
  return { busy: busy || !connected, out, act, clear: () => setOut(null), hint: connected ? '' : 'Connect to the server first' }
}

const Result = ({ out, onClose }: { out: { text: string; bad: boolean } | null; onClose: () => void }) =>
  out && (
    <div className={`act-out ${out.bad ? 'bad' : ''}`}>
      <pre>{out.text}</pre>
      <button className="btn" onClick={onClose}>
        Close
      </button>
    </div>
  )

/** A failed service: its journal, its status, and a restart. */
export function ServiceRow({ unit, onLog, ...s }: Shared & { unit: string; onLog: () => void }) {
  const { busy, out, act, clear, hint } = useAct(s)
  const call = (a: ServiceAct) => () => s.backend.actions.service(s.server.id, unit, a)
  return (
    <div>
      <div className="bad" style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
        <Mark kind="struck" /> {unit}
        <button className="btn" disabled={!s.connected} onClick={onLog} title={hint || `Follow the log of ${unit}`}>
          Log
        </button>
        <button className="btn" disabled={busy} onClick={() => act('Status', unit, call('status'), false)} title={hint || `Show the status of ${unit}`}>
          Status
        </button>
        <button className="btn" disabled={busy} onClick={() => act('Restart', unit, call('restart'), true)} title={hint || `Restart ${unit}`}>
          Restart
        </button>
      </div>
      <Result out={out} onClose={clear} />
    </div>
  )
}

/** The Start/Stop and Restart buttons of one container row. */
export function ContainerActions({ c, ...s }: Shared & { c: Container }) {
  const { busy, out, act, clear, hint } = useAct(s)
  const call = (a: 'start' | 'stop' | 'restart') => () => s.backend.actions.container(s.server.id, c.name, a)
  const running = c.status === 'running'
  return (
    <>
      <button className="btn" disabled={busy} onClick={() => act(running ? 'Stop' : 'Start', c.name, call(running ? 'stop' : 'start'), true)} title={hint || `${running ? 'Stop' : 'Start'} ${c.name}`}>
        {running ? 'Stop' : 'Start'}
      </button>{' '}
      <button className="btn" disabled={busy} onClick={() => act('Restart', c.name, call('restart'), true)} title={hint || `Restart ${c.name}`}>
        Restart
      </button>
      <Result out={out} onClose={clear} />
    </>
  )
}
