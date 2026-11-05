import { useEffect, useState } from 'react'
import { messageOf } from '../lib/config'
import { updateApi, type UpdateInfo, type UpdateStatus } from '../lib/update'

type Phase = 'idle' | 'checking' | 'installing' | 'installed'

/** The running version, a check for a newer release, and installing it. Nothing installs without a click. */
export function UpdatePanel() {
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [info, setInfo] = useState<UpdateInfo | null>(null)
  const [phase, setPhase] = useState<Phase>('idle')
  const [log, setLog] = useState('')
  const [err, setErr] = useState('')

  useEffect(() => {
    updateApi.status().then(setStatus).catch((e) => setErr(messageOf(e)))
  }, [])

  const run = async (next: Phase, f: () => Promise<void>) => {
    setPhase(next)
    setErr('')
    try {
      await f()
    } catch (e) {
      setErr(messageOf(e))
      setPhase('idle')
    }
  }
  const check = () =>
    run('checking', async () => {
      setInfo(await updateApi.check())
      setPhase('idle')
    })
  const install = () =>
    run('installing', async () => {
      await updateApi.install(setLog)
      setPhase('installed')
    })
  const toggle = async (on: boolean) => {
    try {
      await updateApi.setAutoCheck(on)
      setStatus(await updateApi.status())
    } catch (e) {
      setErr(messageOf(e))
    }
  }

  return (
    <div className="admin-form" style={{ borderLeft: 0, maxWidth: 760 }}>
      <h2 className="name">Updates</h2>
      <p>
        You are running <strong>{status?.version ?? '…'}</strong>.
      </p>
      <div className="actions">
        <button className="btn" onClick={check} disabled={phase === 'checking' || phase === 'installing'}>
          {phase === 'checking' ? 'Checking…' : 'Check for updates'}
        </button>
      </div>

      {info && !info.available && phase !== 'installed' && <p className="good">You have the latest version{info.latest ? ` (${info.latest})` : ''}.</p>}
      {info?.available && phase !== 'installed' && (
        <div role="status">
          <p>
            <strong>{info.latest}</strong> is available.
          </p>
          {info.notes && <pre className="deplog" aria-label="Release notes">{info.notes}</pre>}
          <div className="actions">
            {info.canInstall ? (
              <button className="btn solid" onClick={install} disabled={phase === 'installing'}>
                {phase === 'installing' ? 'Installing…' : `Install ${info.latest}`}
              </button>
            ) : (
              <span className="dim">This release cannot be installed from here.</span>
            )}
            <button className="btn" onClick={() => updateApi.openPage(info.pageUrl)}>Open the release page</button>
          </div>
          {phase === 'installing' && log && <p className="dim">{log}</p>}
        </div>
      )}
      {phase === 'installed' && (
        <div role="status">
          <p className="good">Installed {info?.latest}. Restart sshdeck to use it. Open connections will be closed.</p>
          <div className="actions">
            <button className="btn solid" onClick={() => void updateApi.restart().catch((e) => setErr(messageOf(e)))}>Restart now</button>
          </div>
        </div>
      )}
      {err && (
        <p className="bad" role="alert">
          {err}
          {info?.pageUrl && (
            <>
              {' '}
              <button className="btn" onClick={() => updateApi.openPage(info.pageUrl)}>Open the release page</button>
            </>
          )}
        </p>
      )}

      <label style={{ display: 'flex', gap: 8, alignItems: 'center', marginTop: 16 }}>
        <input type="checkbox" checked={status?.autoCheck ?? true} onChange={(e) => void toggle(e.target.checked)} />
        <span>Look for a newer version when sshdeck starts (asks GitHub, sends nothing about you)</span>
      </label>
    </div>
  )
}
