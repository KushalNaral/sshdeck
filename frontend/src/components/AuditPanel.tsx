import { useCallback, useEffect, useMemo, useState } from 'react'
import { auditApi, EVENT_LABEL, type AuditCheck, type AuditEntry } from '../lib/audit'
import { messageOf } from '../lib/config'
import { Mark } from './Mark'

const BAD = new Set(['connect.fail', 'unlock.fail'])

/** What was done through sshdeck, newest first. Read-only; passwords and keystrokes are never recorded. */
export function AuditPanel() {
  const [rows, setRows] = useState<AuditEntry[] | null>(null)
  const [check, setCheck] = useState<AuditCheck | null>(null)
  const [query, setQuery] = useState('')
  const [err, setErr] = useState('')

  const refresh = useCallback(async () => {
    setErr('')
    try {
      const [r, c] = await Promise.all([auditApi.list(300), auditApi.verify()])
      setRows(r)
      setCheck(c)
    } catch (e) {
      setErr(messageOf(e))
    }
  }, [])
  useEffect(() => void refresh(), [refresh])

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase()
    return (rows ?? []).filter((r) => !q || `${EVENT_LABEL[r.event] ?? r.event} ${r.event} ${r.subject ?? ''} ${r.detail ?? ''} ${r.actor}`.toLowerCase().includes(q))
  }, [rows, query])

  return (
    <div className="admin-form" style={{ borderLeft: 0, maxWidth: 1000 }}>
      <div className="tools" style={{ padding: 0, borderBottom: 0 }}>
        <input className="search" type="search" placeholder="Filter by server, event or person" aria-label="Filter the audit log" value={query} onChange={(e) => setQuery(e.target.value)} />
        <button className="btn" onClick={() => void refresh()}>
          Refresh
        </button>
        {check && (
          <span className={check.ok ? 'good' : 'bad'} role="status">
            <Mark kind={check.ok ? 'confirmed' : 'struck'} /> {check.ok ? `Chain intact · ${check.entries} entries in this file` : `Chain broken at line ${check.badLine}: the log was edited`}
          </span>
        )}
      </div>
      {err && (
        <p className="bad" role="alert">
          {err}
        </p>
      )}
      <table>
        <thead>
          <tr>
            <th>When</th>
            <th>Who</th>
            <th>What</th>
            <th>Server or item</th>
            <th>Detail</th>
          </tr>
        </thead>
        <tbody>
          {shown.map((r, i) => (
            <tr key={`${r.ts}-${i}`}>
              <td>{new Date(r.ts).toLocaleString()}</td>
              <td>{r.actor}</td>
              <td className={BAD.has(r.event) ? 'bad' : ''}>
                <strong>{EVENT_LABEL[r.event] ?? r.event}</strong>
              </td>
              <td>{r.subject}</td>
              <td className="dim">{r.detail}</td>
            </tr>
          ))}
          {rows && shown.length === 0 && (
            <tr>
              <td colSpan={5} className="dim">
                {rows.length === 0 ? 'Nothing recorded yet.' : 'No entry matches the filter.'}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}
