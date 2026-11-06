import { useState } from 'react'
import { messageOf } from '../lib/config'
import { importApi, type ImportPreview, type ImportResult } from '../lib/importer'

const TYPE = { vpn: 'VPN', server: 'Server', login: 'Login', '': '?' } as const
const ACTION = { new: 'New', update: 'Update', error: 'Error' } as const

/** Bring servers, VPN profiles and other logins in from a CSV file, or save them out. Passwords stay in Go. */
export function ImportPanel({ onImported }: { onImported: () => void }) {
  const [pv, setPv] = useState<ImportPreview | null>(null)
  const [picked, setPicked] = useState<Set<number>>(new Set())
  const [res, setRes] = useState<ImportResult | null>(null)
  const [note, setNote] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const run = async (f: () => Promise<void>) => {
    setBusy(true)
    setErr('')
    try {
      await f()
    } catch (e) {
      setErr(messageOf(e))
    } finally {
      setBusy(false)
    }
  }
  const choose = (pick: () => Promise<ImportPreview | null> = importApi.pick) =>
    run(async () => {
      const p = await pick()
      if (!p) return
      setPv(p)
      setRes(null)
      setNote('')
      setPicked(new Set(p.rows.filter((r) => r.action !== 'error').map((r) => r.index)))
    })
  const apply = () =>
    run(async () => {
      if (!pv) return
      const r = await importApi.apply(pv.token, [...picked])
      setRes(r)
      setPv(null)
      onImported()
    })
  const saved = (what: string, f: () => Promise<string>) =>
    run(async () => {
      const path = await f()
      setNote(path ? `${what} saved to ${path}` : '')
    })
  const toggle = (i: number) => setPicked((s) => (s.has(i) ? new Set([...s].filter((x) => x !== i)) : new Set(s).add(i)))

  return (
    <div className="admin-form import">
      <h2 className="name">Import and export</h2>
      <p className="dim">
        Use a spreadsheet saved as CSV. Rows with the same name as something you already have update it; a blank password keeps the stored one. Passwords are read by the app itself and are never shown here.
      </p>
      <div className="actions">
        <button className="btn solid" onClick={() => choose()} disabled={busy}>Choose a CSV file…</button>
        <button className="btn" onClick={() => choose(importApi.pickSSHConfig)} disabled={busy} title="Reads Host entries, including ProxyJump. Passwords are not in that file: logins use the key or the ssh-agent.">Read an SSH config…</button>
        <button className="btn" onClick={() => saved('Template', importApi.saveTemplate)} disabled={busy}>Save a blank template…</button>
        <button className="btn" onClick={() => saved('Export', importApi.exportAll)} disabled={busy} title="Everything you have, without passwords">Export all…</button>
      </div>
      {err && <p className="bad" role="alert">{err}</p>}
      {note && <p className="dim" role="status">{note}</p>}

      {pv && (
        <>
          <h3 className="label">Review {pv.file}</h3>
          <table aria-label="Rows in the file">
            <thead>
              <tr><th><span aria-hidden>✓</span></th><th>Line</th><th>Kind</th><th>Name</th><th>What happens</th><th>Notes</th></tr>
            </thead>
            <tbody>
              {pv.rows.map((r) => (
                <tr key={r.index}>
                  <td><input type="checkbox" checked={picked.has(r.index)} disabled={r.action === 'error'} onChange={() => toggle(r.index)} aria-label={`Import line ${r.line}`} /></td>
                  <td>{r.line}</td>
                  <td>{TYPE[r.type]}</td>
                  <td>{r.name || '(no name)'}</td>
                  <td className={r.action === 'error' ? 'bad' : ''}>{ACTION[r.action]}</td>
                  <td>{r.error ?? (r.hasPassword ? 'sets a password' : '')}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="actions">
            <button className="btn solid" onClick={apply} disabled={busy || picked.size === 0}>Import {picked.size} row{picked.size === 1 ? '' : 's'}</button>
            <button className="btn" onClick={() => setPv(null)} disabled={busy}>Cancel</button>
          </div>
        </>
      )}

      {res && (
        <div role="status">
          <p>{res.added} added, {res.updated} updated{res.failures.length ? `, ${res.failures.length} failed` : ''}.</p>
          {res.failures.length > 0 && <ul className="bad">{res.failures.map((f) => <li key={f}>{f}</li>)}</ul>}
        </div>
      )}
    </div>
  )
}
