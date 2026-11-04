import { useMemo, useState } from 'react'
import type { Server, Task, TaskStatus } from '../types'
import { Mark } from './Mark'

const NEXT: Record<TaskStatus, TaskStatus> = { todo: 'doing', doing: 'done', done: 'todo' }
const LABEL: Record<TaskStatus, string> = { todo: 'To do', doing: 'In progress', done: 'Done' }
const MARK = { todo: 'hollow', doing: 'filled', done: 'confirmed' } as const
type Show = 'open' | 'done' | 'all'

const today = () => new Date().toLocaleDateString('en-CA') // YYYY-MM-DD in local time
const splitTags = (s: string) => s.split(',').map((t) => t.trim().toLowerCase()).filter(Boolean)

interface Props {
  tasks: Task[]
  servers: Server[]
  onSave: (t: Task) => Promise<string | null>
  onDelete: (id: string) => Promise<string | null>
  onOpen: (serverId: string) => void
}

const blank = (): Task => ({ id: '', title: '', note: '', status: 'todo', tags: [], serverIds: [], due: '', created: 0, updated: 0 })

/** Work on the fleet: filter by tag, server or text; click a task's mark to move it todo → doing → done. */
export function TasksBoard({ tasks, servers, onSave, onDelete, onOpen }: Props) {
  const [show, setShow] = useState<Show>('open')
  const [query, setQuery] = useState('')
  const [tag, setTag] = useState('')
  const [serverId, setServerId] = useState('')
  const [editing, setEditing] = useState<Task | null>(null)
  const [error, setError] = useState('')
  const byId = useMemo(() => new Map(servers.map((s) => [s.id, s])), [servers])

  const tagCounts = useMemo(() => {
    const m = new Map<string, number>()
    tasks.forEach((t) => t.tags.forEach((g) => m.set(g, (m.get(g) ?? 0) + 1)))
    return [...m].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
  }, [tasks])

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const due = (t: Task) => t.due || '9999'
    return tasks
      .filter((t) => (show === 'all' ? true : show === 'done' ? t.status === 'done' : t.status !== 'done'))
      .filter((t) => !tag || t.tags.includes(tag))
      .filter((t) => !serverId || t.serverIds.includes(serverId))
      .filter((t) => !q || `${t.title} ${t.note ?? ''} ${t.tags.join(' ')}`.toLowerCase().includes(q))
      .sort((a, b) => Number(a.status === 'done') - Number(b.status === 'done') || due(a).localeCompare(due(b)) || b.updated - a.updated)
  }, [tasks, show, query, tag, serverId])

  const run = async (p: Promise<string | null>) => {
    const err = await p
    setError(err ?? '')
    return err === null
  }
  const open = tasks.filter((t) => t.status !== 'done').length
  const overdue = tasks.filter((t) => t.status !== 'done' && t.due && t.due < today()).length

  return (
    <section className="pane board" aria-label="Tasks">
      <div className="tools">
        <strong>
          {open} open{overdue > 0 && <span className="bad"> · {overdue} overdue</span>} · {tasks.length - open} done
        </strong>
        <button className="btn" onClick={() => setEditing(blank())}>
          New task
        </button>
      </div>
      <div className="tools">
        <input className="search" type="search" placeholder="Search tasks" aria-label="Search tasks" value={query} onChange={(e) => setQuery(e.target.value)} />
        {(['open', 'done', 'all'] as Show[]).map((s) => (
          <button key={s} className="btn" aria-pressed={show === s} onClick={() => setShow(s)}>
            {s === 'open' ? 'Open' : s === 'done' ? 'Done' : 'All'}
          </button>
        ))}
        <select className="search" style={{ flex: '0 1 180px' }} aria-label="Server" value={serverId} onChange={(e) => setServerId(e.target.value)}>
          <option value="">Any server</option>
          {servers.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
      </div>
      {tagCounts.length > 0 && (
        <div className="tools picks" role="group" aria-label="Tags">
          {tagCounts.map(([g, n]) => (
            <button key={g} className="chip" aria-pressed={tag === g} onClick={() => setTag(tag === g ? '' : g)}>
              {g} · {n}
            </button>
          ))}
        </div>
      )}
      {error && (
        <p className="bad" role="alert" style={{ padding: '8px 16px' }}>
          {error}
        </p>
      )}
      {editing && <TaskForm key={editing.id || 'new'} task={editing} servers={servers} onCancel={() => setEditing(null)} onSave={async (t) => {
          const ok = await run(onSave(t))
          if (ok) setEditing(null)
          return ok
        }} />}
      {rows.map((t) => {
        const late = t.status !== 'done' && !!t.due && t.due < today()
        return (
          <div key={t.id} className={`trow ${t.status}`}>
            <button className="statusbtn" title={`${LABEL[t.status]}. Click for ${LABEL[NEXT[t.status]]}`} aria-label={`${t.title}: ${LABEL[t.status]}. Mark as ${LABEL[NEXT[t.status]]}`} onClick={() => void run(onSave({ ...t, status: NEXT[t.status] }))}>
              <Mark kind={MARK[t.status]} />
            </button>
            <div>
              <strong className="ttitle">{t.title}</strong>
              {t.note && <div className="dim">{t.note}</div>}
              <div className="meta">
                {t.status === 'done' ? <span className="stamp">Closed</span> : <span>{LABEL[t.status]}</span>}
                {t.due && <span className={late ? 'bad' : ''}>{late ? 'Overdue · ' : 'Due '}{t.due}</span>}
                {t.serverIds.map((id) => (
                  <button key={id} onClick={() => onOpen(id)}>
                    {byId.get(id)?.name ?? id}
                  </button>
                ))}
                {t.tags.map((g) => (
                  <button key={g} onClick={() => setTag(g)}>
                    #{g}
                  </button>
                ))}
              </div>
            </div>
            <span className="picks">
              <button className="btn" onClick={() => setEditing(t)}>
                Edit
              </button>
              <button className="btn" onClick={() => void run(onDelete(t.id))}>
                Delete
              </button>
            </span>
          </div>
        )
      })}
      {rows.length === 0 && <p className="dim" style={{ padding: 16 }}>{tasks.length === 0 ? 'No tasks yet. Add one, for example “Upgrade PHP on Atlas”.' : 'No task matches these filters.'}</p>}
    </section>
  )
}

function TaskForm({ task, servers, onSave, onCancel }: { task: Task; servers: Server[]; onSave: (t: Task) => Promise<boolean>; onCancel: () => void }) {
  const [t, setT] = useState(task)
  const [tags, setTags] = useState(task.tags.join(', '))
  const [busy, setBusy] = useState(false)
  const toggle = (id: string) => setT((p) => ({ ...p, serverIds: p.serverIds.includes(id) ? p.serverIds.filter((x) => x !== id) : [...p.serverIds, id] }))
  return (
    <form
      className="tform"
      onSubmit={async (e) => {
        e.preventDefault()
        setBusy(true)
        await onSave({ ...t, tags: splitTags(tags) })
        setBusy(false)
      }}
    >
      <label className="field">
        <span>Title</span>
        <input autoFocus value={t.title} onChange={(e) => setT({ ...t, title: e.target.value })} placeholder="Upgrade PHP to 8.2.35" />
      </label>
      <label className="field">
        <span>Note</span>
        <textarea value={t.note ?? ''} onChange={(e) => setT({ ...t, note: e.target.value })} />
      </label>
      <div className="two">
        <label className="field">
          <span>Tags (comma separated)</span>
          <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="php, urgent, client-x" />
        </label>
        <label className="field">
          <span>Due</span>
          <input type="date" value={t.due ?? ''} onChange={(e) => setT({ ...t, due: e.target.value })} />
        </label>
      </div>
      <label className="field">
        <span>Status</span>
        <select value={t.status} onChange={(e) => setT({ ...t, status: e.target.value as TaskStatus })}>
          {(Object.keys(LABEL) as TaskStatus[]).map((s) => (
            <option key={s} value={s}>
              {LABEL[s]}
            </option>
          ))}
        </select>
      </label>
      <div className="field">
        <span>Servers</span>
        <div className="picks" role="group" aria-label="Servers">
          {servers.map((s) => (
            <button type="button" key={s.id} className="chip" aria-pressed={t.serverIds.includes(s.id)} onClick={() => toggle(s.id)}>
              {s.name}
            </button>
          ))}
        </div>
      </div>
      <span className="picks">
        <button className="btn" type="submit" disabled={busy || !t.title.trim()}>
          {t.id ? 'Save task' : 'Add task'}
        </button>
        <button className="btn" type="button" onClick={onCancel}>
          Cancel
        </button>
      </span>
    </form>
  )
}
