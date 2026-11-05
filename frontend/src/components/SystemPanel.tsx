import { useCallback, useEffect, useState } from 'react'
import { backend } from '../backend'
import type { Dep } from '../types'
import { messageOf } from '../lib/config'
import { Mark } from './Mark'

/** What this machine needs: the VPN clients and the prompt that asks for your admin password. */
export function SystemPanel() {
  const [deps, setDeps] = useState<Dep[] | null>(null)
  const [log, setLog] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const refresh = useCallback(() => backend.deps().then(setDeps).catch((e) => setErr(messageOf(e))), [])
  useEffect(() => void refresh(), [refresh])

  const missing = (deps ?? []).filter((d) => !d.installed && d.installable)
  const install = async (names: string[]) => {
    setBusy(true)
    setErr(null)
    setLog([])
    try {
      await backend.installDeps(names, (l) => setLog((p) => [...p, l]))
    } catch (e) {
      setErr(messageOf(e))
    }
    setBusy(false)
    void refresh()
  }

  return (
    <div className="admin-form" style={{ borderLeft: 0, maxWidth: 760 }}>
      <h2 className="name">Programs on this machine</h2>
      <table>
        <thead>
          <tr><th /><th>Program</th><th>Used for</th><th /></tr>
        </thead>
        <tbody>
          {(deps ?? []).map((d) => (
            <tr key={d.name}>
              <td><Mark kind={d.installed ? 'filled' : 'struck'} /></td>
              <td><strong className="name">{d.name}</strong></td>
              <td>{d.purpose}</td>
              <td>
                {d.installed ? (
                  <span className="dim">Installed</span>
                ) : d.installable ? (
                  <button className="btn" disabled={busy} onClick={() => void install([d.name])}>Install</button>
                ) : (
                  <span className="bad">Missing: install polkit with your package manager</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="actions" style={{ marginTop: 16 }}>
        <button className="btn solid" disabled={busy || missing.length === 0} onClick={() => void install(missing.map((d) => d.name))}>
          {busy ? 'Installing…' : missing.length ? `Install ${missing.length} missing` : 'Everything is installed'}
        </button>
        <span className="dim">Your system will ask for administrator approval.</span>
      </div>
      {err && <p className="bad" role="alert">{err}</p>}
      {log.length > 0 && <pre className="deplog" aria-label="Install output">{log.join('\n')}</pre>}
    </div>
  )
}
