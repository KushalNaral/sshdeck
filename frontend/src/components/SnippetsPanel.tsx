import { useState } from 'react'
import { messageOf } from '../lib/config'
import { snippetsApi, useSnippets } from '../lib/snippets'
import type { Snippet } from '../types'

const blank = (): Snippet => ({ id: '', name: '', command: '' })

/** Saved commands. They show up in the Run view and in each terminal's Snippets menu. */
export function SnippetsPanel() {
  const { list, reload } = useSnippets()
  const [d, setD] = useState<Snippet | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const save = async () => {
    if (!d) return
    setErr(null)
    try {
      await snippetsApi.save(d)
      setD(null)
      reload()
    } catch (e) {
      setErr(messageOf(e))
    }
  }
  const remove = async (id: string) => {
    try {
      await snippetsApi.remove(id)
      setD(null)
      reload()
    } catch (e) {
      setErr(messageOf(e))
    }
  }
  return (
    <div className="admin-form">
      <h2 className="name">Snippets</h2>
      <p className="dim">Commands you run often. Several lines are fine; the Run view runs them on the servers you pick, and a terminal's Snippets menu types one without running it.</p>
      <ul className="snip-list">
        {list.map((x) => (
          <li key={x.id}>
            <button className="btn" onClick={() => setD(x)} aria-pressed={d?.id === x.id}>{x.name}</button>
            <code className="dim">{x.command.split('\n')[0]}</code>
          </li>
        ))}
      </ul>
      {d === null ? (
        <div className="actions"><button className="btn solid" onClick={() => setD(blank())}>Add snippet</button></div>
      ) : (
        <form onSubmit={(e) => (e.preventDefault(), void save())} className="admin-form">
          <label className="field"><span>Name</span><input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="Disk usage" /></label>
          <label className="field"><span>Command</span><textarea rows={5} spellCheck={false} value={d.command} onChange={(e) => setD({ ...d, command: e.target.value })} placeholder="df -h" /></label>
          <label className="field"><span>Note</span><input value={d.note ?? ''} onChange={(e) => setD({ ...d, note: e.target.value })} /></label>
          {err && <p className="bad" role="alert">{err}</p>}
          <div className="actions">
            <button className="btn solid" type="submit">{d.id ? 'Save changes' : 'Add snippet'}</button>
            <button className="btn" type="button" onClick={() => (setD(null), setErr(null))}>Cancel</button>
            {d.id && <button className="btn" type="button" onClick={() => void remove(d.id)}>Delete</button>}
          </div>
        </form>
      )}
    </div>
  )
}
