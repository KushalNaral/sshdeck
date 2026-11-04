import { useState } from 'react'
import { messageOf } from '../lib/config'
import { lockApi, type LockState } from '../lib/lock'

const MIN_LEN = 8
const IDLE_CHOICES = [0, 5, 15, 30, 60]

interface Props {
  lock: LockState
  onChange: (s: LockState) => void
}

/** Passphrase lock: turn it on, set when it locks itself, change the passphrase. */
export function SecurityPanel({ lock, onChange }: Props) {
  const [a, setA] = useState('')
  const [b, setB] = useState('')
  const [c, setC] = useState('')
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const run = async (fn: () => Promise<string | void>) => {
    setBusy(true)
    setErr('')
    setMsg('')
    try {
      const m = await fn()
      if (m) setMsg(m)
      onChange(await lockApi.state())
      setA('')
      setB('')
      setC('')
    } catch (e) {
      setErr(messageOf(e))
    }
    setBusy(false)
  }
  const checkNew = (p: string, q: string) => (p.length < MIN_LEN ? `Use at least ${MIN_LEN} characters.` : p !== q ? 'The two passphrases do not match.' : '')

  if (!lock.enabled) {
    const problem = a ? checkNew(a, b) : ''
    return (
      <form
        className="admin-form"
        style={{ borderLeft: 0, maxWidth: 560 }}
        onSubmit={(e) => {
          e.preventDefault()
          const p = checkNew(a, b)
          if (p) return setErr(p)
          void run(async () => {
            const left = await lockApi.enable(a)
            return left.length ? `Lock is on, but ${left.length} password(s) could not be removed from the OS keyring. Remove the "sshdeck" entries there by hand.` : undefined
          })
        }}
      >
        <h2 className="name">Protect with a passphrase</h2>
        <p>Encrypts your server list, VPN profiles, tasks and passwords on this computer, and asks for the passphrase when the app starts or has been idle.</p>
        <p className="bad">There is no recovery. If you forget the passphrase, the data cannot be opened.</p>
        <label className="field">
          <span>Passphrase</span>
          <input type="password" autoComplete="new-password" value={a} onChange={(e) => setA(e.target.value)} />
        </label>
        <label className="field">
          <span>Repeat passphrase</span>
          <input type="password" autoComplete="new-password" value={b} onChange={(e) => setB(e.target.value)} />
        </label>
        {(problem || err) && (
          <p className="bad" role="alert">
            {err || problem}
          </p>
        )}
        {msg && <p className="good">{msg}</p>}
        <div className="actions">
          <button className="btn solid" type="submit" disabled={busy || !a}>
            {busy ? 'Encrypting…' : 'Turn on lock'}
          </button>
        </div>
      </form>
    )
  }

  const save = (idle: number, end: boolean) => void run(async () => void (await lockApi.settings(idle, end)))
  const changeProblem = c ? checkNew(b, c) : ''
  return (
    <div className="admin-form" style={{ borderLeft: 0, maxWidth: 560 }}>
      <h2 className="name">Lock</h2>
      <p className="good">The passphrase lock is on. Your data is encrypted on this computer.</p>
      <div className="actions">
        <button className="btn solid" onClick={() => void lockApi.lock()}>
          Lock now
        </button>
      </div>
      <label className="field" style={{ marginTop: 16 }}>
        <span>Lock after being idle for</span>
        <select value={lock.idleMinutes} onChange={(e) => save(Number(e.target.value), lock.endOnLock)}>
          {IDLE_CHOICES.map((m) => (
            <option key={m} value={m}>
              {m === 0 ? 'Never' : `${m} minutes`}
            </option>
          ))}
        </select>
      </label>
      <label style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 16 }}>
        <input type="checkbox" checked={lock.endOnLock} onChange={(e) => save(lock.idleMinutes, e.target.checked)} />
        <span>End open shells and VPNs when it locks (otherwise they keep running)</span>
      </label>

      <h2 className="name">Change passphrase</h2>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          const p = checkNew(b, c)
          if (p) return setErr(p)
          void run(async () => {
            await lockApi.change(a, b)
            return 'Passphrase changed.'
          })
        }}
      >
        <label className="field">
          <span>Current passphrase</span>
          <input type="password" autoComplete="current-password" value={a} onChange={(e) => setA(e.target.value)} />
        </label>
        <label className="field">
          <span>New passphrase</span>
          <input type="password" autoComplete="new-password" value={b} onChange={(e) => setB(e.target.value)} />
        </label>
        <label className="field">
          <span>Repeat new passphrase</span>
          <input type="password" autoComplete="new-password" value={c} onChange={(e) => setC(e.target.value)} />
        </label>
        {(changeProblem || err) && (
          <p className="bad" role="alert">
            {err || changeProblem}
          </p>
        )}
        {msg && <p className="good">{msg}</p>}
        <div className="actions">
          <button className="btn" type="submit" disabled={busy || !a || !b}>
            Change passphrase
          </button>
        </div>
      </form>
    </div>
  )
}
