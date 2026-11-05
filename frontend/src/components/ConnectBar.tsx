import type { Mark as MarkKind, Server, Session, Step, Vpn } from '../types'
import { Mark } from './Mark'

const stepMark = (s: Step): MarkKind => (s.status === 'done' ? 'confirmed' : s.status === 'fail' ? 'struck' : 'hollow')
const secs = (ms?: number) => (ms === undefined ? '' : `${(ms / 1000).toFixed(1)}s`)

interface Props {
  server: Server
  vpn?: Vpn
  session?: Session
  onConnect: () => void
  onDisconnect: () => void
  onManualDone: () => void
}

/** The path to the shell, one cell per step, each as wide as it actually took. */
export function ConnectBar({ server, vpn, session, onConnect, onDisconnect, onManualDone }: Props) {
  const steps = session?.steps ?? []
  const manual = steps.find((s) => s.status === 'manual')
  return (
    <section className="connect glassbar" aria-label="Connection" aria-live="polite">
      {steps.length > 0 ? (
        <div className="steps">
          {steps.map((s) => (
            <div key={s.key} className="step" data-s={s.status} style={{ flexGrow: s.ms ? Math.max(1, Math.round(s.ms / 300)) : 1 }}>
              <Mark kind={stepMark(s)} />
              <span className="label">{s.label}</span>
              {s.detail && <span className="trunc">{s.detail}</span>}
              <span>{s.status === 'run' ? 'working…' : s.status === 'manual' ? 'waiting for you' : secs(s.ms)}</span>
            </div>
          ))}
        </div>
      ) : (
        <div className="msg">
          <Mark kind="hollow" />
          <span className="name">{server.name}</span>
          <span>{vpn ? `Not connected. ${vpn.name} comes up first.` : server.vpnRequired ? 'Not connected. This server needs a VPN, but none is set.' : 'Not connected. Straight to SSH.'}</span>
        </div>
      )}

      <div className="msg" style={{ marginLeft: 'auto' }}>
        {server.production && <span className="prod" title="Production server: be careful what you run">PRODUCTION</span>}
        {session?.state === 'live' && (
          <>
            {session.link?.state === 'reconnecting' ? (
              <span className="label" role="status">Reconnecting… {session.link.attempt}/{session.link.max}</span>
            ) : session.link?.state === 'lost' ? (
              <span className="bad" role="alert"><strong>{session.link.problem}</strong> {session.link.recovery}</span>
            ) : (
              <span className="good label">Live</span>
            )}
            <button className="btn" onClick={onDisconnect}>
              Disconnect
            </button>
          </>
        )}
        {session?.state === 'failed' && (
          <>
            <span className="bad">
              <strong>{session.error?.problem}</strong> {session.error?.recovery}
            </span>
            <button className="btn primary" onClick={onConnect}>
              Try again
            </button>
          </>
        )}
        {manual && (
          <>
            <span>Start {vpn?.name ?? 'the VPN'} yourself, then confirm.</span>
            <button className="btn primary" onClick={onManualDone}>
              I'm connected
            </button>
          </>
        )}
        {session?.state === 'connecting' && (
          <button className="btn" onClick={onDisconnect}>
            Cancel
          </button>
        )}
        {!session && (
          <button className="btn primary" onClick={onConnect}>
            Connect
          </button>
        )}
      </div>
    </section>
  )
}
