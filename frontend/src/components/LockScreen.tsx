import { useState } from 'react'

interface Props {
  /** Resolves with an error message, or null when the app unlocked. */
  onUnlock: (pass: string) => Promise<string | null>
}

/** Covers everything until the passphrase is entered. */
export function LockScreen({ onUnlock }: Props) {
  const [pass, setPass] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  return (
    <div className="lockscreen" role="dialog" aria-modal="true" aria-label="sshdeck is locked">
      <form
        className="lockcard"
        onSubmit={async (e) => {
          e.preventDefault()
          if (!pass || busy) return
          setBusy(true)
          const m = await onUnlock(pass)
          setBusy(false)
          if (m) {
            setErr(m)
            setPass('')
          }
        }}
      >
        <h1 className="brand">sshdeck is locked</h1>
        <p className="dim">Enter your passphrase to continue. Open sessions keep running while it is locked.</p>
        <label className="field">
          <span>Passphrase</span>
          <input type="password" autoFocus autoComplete="current-password" value={pass} onChange={(e) => setPass(e.target.value)} />
        </label>
        {err && (
          <p className="bad" role="alert">
            {err}
          </p>
        )}
        <button className="btn solid" type="submit" disabled={busy || !pass}>
          {busy ? 'Unlocking…' : 'Unlock'}
        </button>
      </form>
    </div>
  )
}
