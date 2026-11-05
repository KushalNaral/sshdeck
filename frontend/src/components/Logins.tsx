import { useState } from 'react'
import { LOGIN_KIND, type Login, type Server } from '../types'

/** A server's other logins in the details pane. Copy puts the user or password on the clipboard; the password is never shown. */
export function Logins({ server, onCopy }: { server: Server; onCopy: (l: Login, field: 'user' | 'secret') => Promise<string | null> }) {
  const [status, setStatus] = useState<{ ok: boolean; text: string } | null>(null)
  const logins = server.logins ?? []
  if (!logins.length) return null
  const copy = async (l: Login, field: 'user' | 'secret') => {
    const m = await onCopy(l, field)
    setStatus(m ? { ok: false, text: m } : { ok: true, text: field === 'secret' ? `Password for ${l.label} copied. It clears in 30 seconds.` : `User for ${l.label} copied.` })
  }
  return (
    <div className="sect" aria-label="Other logins">
      <h3 className="label">Other logins</h3>
      <ul className="logins">
        {logins.map((l) => (
          <li key={l.id}>
            <span className="tag-kind">{LOGIN_KIND[l.kind]}</span>
            <span className="login-name">
              <b>{l.label}</b>
              {(l.user || l.address) && <span className="dim"> {[l.user, l.address].filter(Boolean).join(' · ')}</span>}
            </span>
            <button className="btn" onClick={() => copy(l, 'user')} disabled={!l.user} aria-label={`Copy user for ${l.label}`}>User</button>
            <button className="btn" onClick={() => copy(l, 'secret')} aria-label={`Copy password for ${l.label}`} title="A clipboard manager may keep a copy after it clears">Password</button>
          </li>
        ))}
      </ul>
      <p className={status && !status.ok ? 'bad' : 'dim'} role="status" style={{ minHeight: '1.4em' }}>{status?.text ?? ''}</p>
    </div>
  )
}
