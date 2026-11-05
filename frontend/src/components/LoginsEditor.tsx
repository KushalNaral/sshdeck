import { useState } from 'react'
import { LOGIN_KIND, type Login, type LoginKind } from '../types'

const blank = (): Login => ({ id: '', label: '', kind: 'db', user: '', address: '' })

/** Add, edit and delete a server's other logins. Saved straight away, separately from the server form. */
export function LoginsEditor({ logins, onSave, onDelete }: { logins: Login[]; onSave: (l: Login, secret: string) => Promise<string | null>; onDelete: (id: string) => Promise<string | null> }) {
  const [draft, setDraft] = useState<Login | null>(null)
  const [pw, setPw] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [asking, setAsking] = useState<string | null>(null)

  const save = async () => {
    if (!draft) return
    if (!draft.label.trim()) return setErr('Label is required. For example: Main database.')
    const m = await onSave({ ...draft, label: draft.label.trim() }, pw)
    if (m) return setErr(m)
    setDraft(null)
    setPw('')
    setErr(null)
  }
  const enter = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault() // this sits inside the server form: Enter must not save the server
      void save()
    }
  }
  const set = <K extends keyof Login>(k: K, v: Login[K]) => setDraft((p) => (p ? { ...p, [k]: v } : p))

  return (
    <fieldset className="logins-editor">
      <legend>Other logins</legend>
      {logins.map((l) => (
        <div key={l.id} className="login-row">
          <span className="tag-kind">{LOGIN_KIND[l.kind]}</span>
          <span style={{ flex: 1 }}>{l.label}{l.user ? <span className="dim"> · {l.user}</span> : null}</span>
          <button className="btn" type="button" onClick={() => (setDraft(l), setPw(''), setErr(null))}>Edit</button>
          {asking === l.id ? (
            <>
              <button className="btn danger" type="button" onClick={async () => { const m = await onDelete(l.id); setAsking(null); if (m) setErr(m) }}>Delete {l.label}</button>
              <button className="btn" type="button" onClick={() => setAsking(null)}>Keep</button>
            </>
          ) : (
            <button className="btn" type="button" onClick={() => setAsking(l.id)}>Delete…</button>
          )}
        </div>
      ))}
      {draft ? (
        <div className="login-draft" onKeyDown={enter}>
          <div className="two">
            <label className="field"><span>Label</span><input value={draft.label} onChange={(e) => set('label', e.target.value)} placeholder="Main database" autoFocus /></label>
            <label className="field">
              <span>Kind</span>
              <select value={draft.kind} onChange={(e) => set('kind', e.target.value as LoginKind)}>
                {(Object.keys(LOGIN_KIND) as LoginKind[]).map((k) => <option key={k} value={k}>{LOGIN_KIND[k]}</option>)}
              </select>
            </label>
          </div>
          <div className="two">
            <label className="field"><span>User</span><input value={draft.user ?? ''} onChange={(e) => set('user', e.target.value)} autoComplete="off" /></label>
            <label className="field"><span>Address</span><input value={draft.address ?? ''} onChange={(e) => set('address', e.target.value)} placeholder="127.0.0.1:3306 or https://…" /></label>
          </div>
          <label className="field">
            <span>Password</span>
            <input type="password" autoComplete="new-password" value={pw} onChange={(e) => setPw(e.target.value)} placeholder={draft.id ? 'Leave blank to keep the stored password' : 'Stored like the server password'} />
          </label>
          {err && <p className="bad" role="alert">{err}</p>}
          <div className="actions">
            <button className="btn solid" type="button" onClick={save}>{draft.id ? 'Save login' : 'Add login'}</button>
            <button className="btn" type="button" onClick={() => (setDraft(null), setErr(null))}>Cancel</button>
          </div>
        </div>
      ) : (
        <div>
          {err && <p className="bad" role="alert">{err}</p>}
          <button className="btn" type="button" onClick={() => (setDraft(blank()), setPw(''), setErr(null))}>Add login</button>
        </div>
      )}
    </fieldset>
  )
}
