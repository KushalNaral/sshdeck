import { useEffect, useState } from 'react'
import { backend } from '../backend'
import { messageOf } from '../lib/config'
import type { HistorySettings, Server } from '../types'
import { Confirm } from './Confirm'

/** How long file history is kept, whether it is copied to servers, and how to clear it. */
export function HistorySettingsPanel({ servers }: { servers: Server[] }) {
  const [s, setS] = useState<HistorySettings | null>(null)
  const [deny, setDeny] = useState('')
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')
  const [clear, setClear] = useState<Server | null>(null)
  const [target, setTarget] = useState(servers[0]?.id ?? '')

  useEffect(() => {
    void backend.history.settings().then((h) => (setS(h), setDeny(h.deny.join(', ')))).catch((e) => setErr(messageOf(e)))
  }, [])

  const save = async () => {
    if (!s) return
    setErr('')
    setMsg('')
    try {
      await backend.history.saveSettings({ ...s, deny: deny.split(',').map((x) => x.trim()).filter(Boolean) })
      setMsg('Saved.')
    } catch (e) {
      setErr(messageOf(e))
    }
  }
  if (!s) return <div className="admin-form dim" style={{ borderLeft: 0 }}>{err || 'Reading settings…'}</div>
  const num = (k: 'keep' | 'days') => (e: React.ChangeEvent<HTMLInputElement>) => setS({ ...s, [k]: Number(e.target.value) })
  return (
    <div className="admin-form" style={{ borderLeft: 0, maxWidth: 640 }}>
      <h2 className="name">File history</h2>
      <p className="dim">Each save in the editor keeps the text it replaced, so a change can be compared and put back. The first time a file is opened or saved, it is kept as it is.</p>
      <label style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        <input type="checkbox" checked={!s.off} onChange={(e) => setS({ ...s, off: !e.target.checked })} />
        <span>Keep file history</span>
      </label>
      <label style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        <input type="checkbox" checked={!s.noServerCopy} onChange={(e) => setS({ ...s, noServerCopy: !e.target.checked })} disabled={s.off} />
        <span>Also keep a copy on the server, in <code>~/.sshdeck/history</code>, so teammates see each other’s edits (production servers need it turned on in the file’s history panel)</span>
      </label>
      <div className="row2">
        <label className="field">
          <span>Versions kept per file</span>
          <input type="number" min={1} max={1000} value={s.keep} onChange={num('keep')} />
        </label>
        <label className="field">
          <span>Days to keep them</span>
          <input type="number" min={1} max={3650} value={s.days} onChange={num('days')} />
        </label>
      </div>
      <p className="dim">The newest version and pinned versions are always kept.</p>
      <label className="field">
        <span>Never record files named (comma separated; key files such as *.pem and id_rsa* are always skipped)</span>
        <input value={deny} onChange={(e) => setDeny(e.target.value)} placeholder="*.sql, secrets.*" />
      </label>
      <div className="actions">
        <button className="btn solid" onClick={() => void save()}>
          Save
        </button>
      </div>
      <h3>Delete history</h3>
      <p className="dim">Old versions can contain passwords that were later changed. This removes them here and from the copy on the server.</p>
      <div className="actions">
        <select value={target} onChange={(e) => setTarget(e.target.value)} aria-label="Server">
          {servers.map((x) => (
            <option key={x.id} value={x.id}>
              {x.name}
            </option>
          ))}
        </select>
        <button className="btn danger" onClick={() => setClear(servers.find((x) => x.id === target) ?? null)} disabled={!target}>
          Delete all history of this server
        </button>
      </div>
      {msg && <p className="good" role="status">{msg}</p>}
      {err && <p className="bad" role="alert">{err}</p>}
      {clear && (
        <Confirm
          title={`Delete all file history for ${clear.name}?`}
          body="Every saved version of every file on this server is removed, here and from its copy on the server. This cannot be undone."
          confirmLabel="Delete history"
          onCancel={() => setClear(null)}
          onConfirm={() => {
            const sv = clear
            setClear(null)
            void backend.history.remove(sv.id, '').then(() => setMsg(`History of ${sv.name} deleted.`)).catch((e) => setErr(messageOf(e)))
          }}
        />
      )}
    </div>
  )
}
