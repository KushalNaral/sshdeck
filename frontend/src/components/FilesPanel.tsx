import { ArrowDown, ArrowUp, History, ScrollText, Search, SquareTerminal, Download, File as FileIcon, FilePlus, FileText, Folder, FolderPlus, FolderUp, Home, Link2, Pencil, RefreshCw, Trash2, Upload, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Backend } from '../api'
import { basename, crumbs, dirname, humanSize, join, shortDate, targetOf, validName } from '../lib/files'
import { messageOf } from '../lib/config'
import type { FileEntry, Listing, Policy, SearchHit, StartResult, Transfer } from '../types'
import { clamp, LIMITS, DEFAULT, type Layout } from '../lib/layout'
import { Confirm } from './Confirm'
import { Splitter } from './Splitter'

interface Props {
  serverId: string
  serverName: string
  live: boolean
  backend: Backend
  /** Opens a file in the editor. target is the server id, or "serverId@container". */
  onEdit: (target: string, path: string, line?: number) => void
  /** Follows a file as it grows. target is as for onEdit. */
  onTail: (target: string, path: string) => void
  /** Opens a shell in this folder. ctr is the container being browsed, or empty for the server. */
  onTerminal: (ctr: string, dir: string) => void
  /** Opens the file in the editor with its history showing. */
  onHistory: (target: string, path: string) => void
  layout: Layout
  onLayout: (f: (l: Layout) => Layout) => void
}

// Where each server's panel was left, so switching servers and back does not lose your place.
const lastDir = new Map<string, string>()

type Ask = { kind: 'up' | 'down'; res: StartResult; dir: string; remotes: string[] } | null
const TEXTY = /\.(txt|md|json|ya?ml|toml|ini|conf|cnf|env|log|php|js|ts|tsx|jsx|css|scss|html?|xml|sh|bash|py|rb|go|rs|java|sql|service|timer|htaccess|gitignore|lock)$|^\.|^[A-Z][A-Za-z]*file$/i

type SortKey = 'name' | 'size' | 'mtime'

// The container each server's panel was showing (empty: the server itself).
const lastCtr = new Map<string, string>()

export function FilesPanel({ serverId: baseId, serverName, live, backend, onEdit, onTail, onTerminal, onHistory, layout, onLayout }: Props) {
  const [ctr, setCtrState] = useState(lastCtr.get(baseId) ?? '')
  const [ctrs, setCtrs] = useState<string[]>([])
  useEffect(() => setCtrState(lastCtr.get(baseId) ?? ''), [baseId])
  useEffect(() => {
    setCtrs([])
    if (live) void backend.files.containers(baseId).then(setCtrs).catch(() => setCtrs([]))
  }, [backend, baseId, live])
  const serverId = targetOf(baseId, ctr) // from here on, "serverId" is where the files are
  const [sort, setSort] = useState<{ key: SortKey; asc: boolean }>({ key: 'name', asc: true })
  const [list, setList] = useState<Listing | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [sel, setSel] = useState<string[]>([])
  const [anchor, setAnchor] = useState('')
  const [editing, setEditing] = useState<{ path: string; isNew?: 'dir' | 'file' } | null>(null)
  const [pathText, setPathText] = useState('')
  const [ask, setAsk] = useState<Ask>(null)
  const [del, setDel] = useState<string[] | null>(null)
  const [transfers, setTransfers] = useState<Transfer[]>([])
  const [msg, setMsg] = useState('')
  const [over, setOver] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const cwd = list?.path ?? ''

  const go = useCallback(
    async (dir: string) => {
      setLoading(true)
      setError('')
      try {
        const l = await backend.files.list(serverId, dir)
        setList(l)
        setPathText(l.path)
        setSel([])
        lastDir.set(serverId, l.path)
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      }
      setLoading(false)
    },
    [backend, serverId],
  )

  useEffect(() => {
    setList(null)
    setSel([])
    if (live) void go(lastDir.get(serverId) ?? '')
  }, [serverId, live, go])

  useEffect(() => backend.files.onTransfers((t) => setTransfers(t.filter((x) => !x.serverId || x.serverId === serverId))), [backend, serverId])

  // Refresh the listing when an upload into this folder finishes.
  const doneCount = transfers.filter((t) => t.up && t.state === 'done').length
  const seen = useRef(0)
  useEffect(() => {
    if (doneCount > seen.current && cwd) void backend.files.list(serverId, cwd).then(setList).catch(() => {})
    seen.current = doneCount
  }, [doneCount, cwd, backend, serverId])

  // Folders stay first whatever the order, like Explorer.
  const entries = useMemo(() => {
    const cmp = (a: FileEntry, b: FileEntry) =>
      sort.key === 'size' ? a.size - b.size : sort.key === 'mtime' ? a.mtime - b.mtime : a.name.toLowerCase().localeCompare(b.name.toLowerCase())
    return [...(list?.entries ?? [])].sort((a, b) => Number(b.dir) - Number(a.dir) || (sort.asc ? 1 : -1) * cmp(a, b))
  }, [list, sort])
  const byPath = useMemo(() => new Map(entries.map((e) => [e.path, e])), [entries])
  const chosen = sel.map((p) => byPath.get(p)).filter((e): e is FileEntry => !!e)
  const flash = (m: string) => {
    setMsg(m)
    setTimeout(() => setMsg((cur) => (cur === m ? '' : cur)), 4000)
  }
  const attempt = async (f: () => Promise<unknown>) => {
    try {
      await f()
      return true
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      return false
    }
  }

  // ---- transfers ----
  // Every transfer starts with policy "ask": if names clash, nothing starts until the dialog is answered.
  const begin = async (kind: 'up' | 'down', dir: string, remotes: string[], first: () => Promise<StartResult>) => {
    setError('')
    try {
      const r = await first()
      if (r.conflicts?.length) setAsk({ kind, res: r, dir, remotes })
      else if (r.started) flash(`${r.started} ${kind === 'up' ? 'upload' : 'download'}${r.started > 1 ? 's' : ''} started`)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }
  const upload = (folder: boolean) => void begin('up', cwd, [], () => backend.files.pickUpload(serverId, cwd, folder))
  const dropUpload = (paths: string[], dir: string) => void begin('up', dir, [], () => backend.files.upload(serverId, dir, paths, 'ask'))
  const download = () => {
    const remotes = chosen.map((e) => e.path)
    void begin('down', '', remotes, () => backend.files.pickDownload(serverId, remotes))
  }
  const resolve = async (p: Policy | 'cancel') => {
    const a = ask
    setAsk(null)
    if (!a || p === 'cancel') return
    try {
      const r =
        a.kind === 'up'
          ? await backend.files.upload(serverId, a.dir, a.res.paths ?? [], p)
          : await backend.files.download(serverId, a.remotes, a.res.dest ?? '', p)
      if (r.started) flash(`${r.started} started`)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  // Files dragged in from the desktop: onto a folder row, or anywhere on the panel for the open folder.
  useEffect(
    () =>
      backend.files.onDrop(({ x, y, paths }) => {
        setOver(false)
        const r = box.current?.getBoundingClientRect()
        if (!live || !r || x < r.left || x > r.right || y < r.top || y > r.bottom) return
        const row = (document.elementFromPoint(x, y) as HTMLElement | null)?.closest<HTMLElement>('[data-dir-path]')
        dropUpload(paths, row?.dataset.dirPath ?? cwd)
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [backend, live, cwd, serverId],
  )
  useEffect(() => {
    const on = () => setOver(!!box.current?.classList.contains('wails-drop-target-active'))
    const mo = new MutationObserver(on)
    if (box.current) mo.observe(box.current, { attributes: true, attributeFilter: ['class'] })
    return () => mo.disconnect()
  }, [live])

  // ---- actions ----
  const [find, setFind] = useState<{ q: string; content: boolean; hits: SearchHit[] | null; busy: boolean; err: string } | null>(null)
  const toggleFind = () => setFind((f) => (f ? null : { q: '', content: false, hits: null, busy: false, err: '' }))
  const runFind = async () => {
    if (!find?.q.trim()) return
    setFind((f) => f && { ...f, busy: true, err: '', hits: null })
    try {
      const hits = await backend.files.search(serverId, cwd, find.q, find.content)
      setFind((f) => f && { ...f, busy: false, hits })
    } catch (e) {
      setFind((f) => f && { ...f, busy: false, err: messageOf(e) })
    }
  }
  const openHit = (h: SearchHit) => (h.line || TEXTY.test(basename(h.path)) ? onEdit(serverId, h.path, h.line) : void go(dirname(h.path)))
  const open = (e: FileEntry) => (e.dir ? void go(e.path) : onEdit(serverId, e.path))
  const startNew = (isNew: 'dir' | 'file') => setEditing({ path: join(cwd, ''), isNew })
  const commitEdit = async (name: string) => {
    const ed = editing
    setEditing(null)
    if (!ed) return
    const bad = validName(name)
    if (bad) return bad && setError(bad)
    const target = join(cwd, name)
    const ok = await attempt(() => (ed.isNew === 'dir' ? backend.files.mkdir(serverId, target) : ed.isNew === 'file' ? backend.files.newFile(serverId, target) : backend.files.rename(serverId, ed.path, target)))
    if (ok) {
      await go(cwd)
      setSel([target])
      if (ed.isNew === 'file') onEdit(serverId, target)
    }
  }
  const remove = async () => {
    const paths = del ?? []
    setDel(null)
    if (await attempt(() => backend.files.remove(serverId, paths))) {
      flash(`Deleted ${paths.length} item${paths.length > 1 ? 's' : ''}`)
      void go(cwd)
    }
  }

  const click = (e: React.MouseEvent, f: FileEntry) => {
    if (e.shiftKey && anchor) {
      const a = entries.findIndex((x) => x.path === anchor)
      const b = entries.findIndex((x) => x.path === f.path)
      setSel(entries.slice(Math.min(a, b), Math.max(a, b) + 1).map((x) => x.path))
    } else if (e.ctrlKey || e.metaKey) {
      setSel((s) => (s.includes(f.path) ? s.filter((p) => p !== f.path) : [...s, f.path]))
      setAnchor(f.path)
    } else {
      setSel([f.path])
      setAnchor(f.path)
    }
  }
  const onKey = (e: React.KeyboardEvent) => {
    if (editing || (e.target as HTMLElement).tagName === 'INPUT') return
    const i = entries.findIndex((x) => x.path === sel[sel.length - 1])
    const move = (n: number) => {
      const f = entries[Math.max(0, Math.min(entries.length - 1, n))]
      if (!f) return
      e.preventDefault()
      if (e.shiftKey && anchor) {
        const a = entries.findIndex((x) => x.path === anchor)
        const b = entries.indexOf(f)
        setSel(entries.slice(Math.min(a, b), Math.max(a, b) + 1).map((x) => x.path))
      } else (setSel([f.path]), setAnchor(f.path))
      box.current?.querySelector(`[data-path="${CSS.escape(f.path)}"]`)?.scrollIntoView({ block: 'nearest' })
    }
    if (e.key === 'ArrowDown') move(i + 1)
    else if (e.key === 'ArrowUp') move(i < 0 ? 0 : i - 1)
    else if (e.key === 'Enter' && chosen.length === 1) open(chosen[0])
    else if (e.key === 'Backspace') (e.preventDefault(), void go(dirname(cwd)))
    else if (e.key === 'F2' && chosen.length === 1) setEditing({ path: chosen[0].path })
    else if (e.key === 'Delete' && chosen.length) setDel(chosen.map((c) => c.path))
    else if (e.key === 'a' && e.ctrlKey) (e.preventDefault(), setSel(entries.map((x) => x.path)))
  }

  if (!live)
    return (
      <div className="files">
        <div className="files-empty">
          <strong>Files appear once {serverName} is connected</strong>
          <span>Connect, then browse, upload, download and edit files here.</span>
        </div>
      </div>
    )

  const noPick = chosen.length === 0
  const by = (key: SortKey) => setSort((o) => ({ key, asc: o.key === key ? !o.asc : true }))
  const arrow = (key: SortKey) => (sort.key === key ? sort.asc ? <ArrowUp size={11} strokeWidth={2.5} /> : <ArrowDown size={11} strokeWidth={2.5} /> : null)
  const active = transfers.filter((t) => t.state === 'queued' || t.state === 'running')
  return (
    <div className={`files${over ? ' dropping' : ''}`} ref={box} style={{ '--wails-drop-target': 'drop', '--c-size': `${layout.cols.size}px`, '--c-mod': `${layout.cols.modified}px` } as React.CSSProperties} onKeyDown={(e) => (e.ctrlKey && e.shiftKey && e.key.toLowerCase() === 's' ? (e.preventDefault(), toggleFind()) : onKey(e))}>
      {(ctrs.length > 0 || ctr) && (
        <label className="files-loc">
          <span>Files in</span>
          <select
            value={ctr}
            aria-label="Where to look: the server or one of its containers"
            onChange={(e) => {
              lastCtr.set(baseId, e.target.value)
              setCtrState(e.target.value)
            }}
          >
            <option value="">{serverName} (the server)</option>
            {[...new Set([...ctrs, ...(ctr ? [ctr] : [])])].map((c) => (
              <option key={c} value={c}>
                Container {c}
              </option>
            ))}
          </select>
        </label>
      )}
      <div className="files-bar">
        <button className="btn icon" onClick={() => void go(dirname(cwd))} disabled={!cwd || cwd === '/'} title="Up one folder (Backspace)" aria-label="Up one folder">
          <ArrowUp size={14} strokeWidth={2} />
        </button>
        <button className="btn icon" onClick={() => void go('')} title="Home folder" aria-label="Home folder">
          <Home size={14} strokeWidth={2} />
        </button>
        <button className="btn icon" onClick={() => void go(cwd)} title="Refresh" aria-label="Refresh">
          <RefreshCw size={14} strokeWidth={2} className={loading ? 'spin' : undefined} />
        </button>
        <form
          className="addr"
          onSubmit={(e) => {
            e.preventDefault()
            void go(pathText.trim())
          }}
        >
          <input value={pathText} onChange={(e) => setPathText(e.target.value)} aria-label="Folder path" spellCheck={false} />
        </form>
      </div>
      <nav className="crumbs" aria-label="Folder path">
        {crumbs(cwd).map((c, i, a) => (
          <button key={c.path} onClick={() => void go(c.path)} aria-current={i === a.length - 1 ? 'page' : undefined}>
            {c.name}
          </button>
        ))}
      </nav>
      <div className="files-cmd" role="toolbar" aria-label="File actions">
        <button className="btn" onClick={() => upload(false)} title="Upload files from this computer">
          <Upload size={14} strokeWidth={2} /> Upload
        </button>
        <button className="btn icon" onClick={() => upload(true)} title="Upload a folder" aria-label="Upload a folder">
          <FolderUp size={14} strokeWidth={2} />
        </button>
        <button className="btn" onClick={download} disabled={noPick} title="Download the selection">
          <Download size={14} strokeWidth={2} /> Download
        </button>
        <span className="sep" />
        <button className="btn icon" onClick={() => startNew('dir')} title="New folder" aria-label="New folder">
          <FolderPlus size={14} strokeWidth={2} />
        </button>
        <button className="btn icon" onClick={() => startNew('file')} title="New file" aria-label="New file">
          <FilePlus size={14} strokeWidth={2} />
        </button>
        <button className="btn icon" onClick={() => setEditing({ path: chosen[0].path })} disabled={chosen.length !== 1} title="Rename (F2)" aria-label="Rename">
          <Pencil size={14} strokeWidth={2} />
        </button>
        <button className="btn icon" onClick={() => onHistory(serverId, chosen[0].path)} disabled={chosen.length !== 1 || chosen[0].dir} title="History of the selected file" aria-label="History">
          <History size={14} strokeWidth={2} />
        </button>
        <button className="btn icon" onClick={() => setDel(chosen.map((c) => c.path))} disabled={noPick} title="Delete (Del)" aria-label="Delete">
          <Trash2 size={14} strokeWidth={2} />
        </button>
        <span className="sep" />
        <button className="btn icon" onClick={toggleFind} aria-pressed={!!find} title="Search this folder (Ctrl+Shift+S)" aria-label="Search">
          <Search size={14} strokeWidth={2} />
        </button>
        <button className="btn icon" onClick={() => onTail(serverId, chosen[0].path)} disabled={chosen.length !== 1 || chosen[0].dir} title="Follow the selected file, like tail -f" aria-label="Follow file">
          <ScrollText size={14} strokeWidth={2} />
        </button>
        <button className="btn icon" onClick={() => onTerminal(ctr, cwd)} disabled={!cwd} title="Open a shell in this folder" aria-label="Terminal here">
          <SquareTerminal size={14} strokeWidth={2} />
        </button>
      </div>
      {find && (
        <div className="files-find" role="search" aria-label={`Search under ${cwd}`}>
          <div className="files-find-bar">
            <input
              autoFocus
              aria-label="Search for"
              placeholder={find.content ? 'Text inside files' : 'File name (* works)'}
              value={find.q}
              onChange={(e) => setFind({ ...find, q: e.target.value })}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void runFind()
                if (e.key === 'Escape') setFind(null)
              }}
            />
            <select aria-label="Search by" value={find.content ? 'content' : 'name'} onChange={(e) => setFind({ ...find, content: e.target.value === 'content', hits: null })}>
              <option value="name">By name</option>
              <option value="content">By content</option>
            </select>
            <button className="btn" onClick={() => void runFind()} disabled={find.busy || !find.q.trim()}>{find.busy ? 'Searching…' : 'Search'}</button>
          </div>
          {find.err && <p className="bad" role="alert">{find.err}</p>}
          {find.hits && (
            <>
              <p className="dim">{find.hits.length === 0 ? 'Nothing found under this folder.' : `${find.hits.length}${find.hits.length >= 500 ? '+' : ''} found under ${cwd}`}</p>
              <ul className="files-hits">
                {find.hits.map((h, i) => (
                  <li key={`${h.path}:${h.line ?? 0}:${i}`}>
                    <button onClick={() => openHit(h)} title={h.path}>
                      <span className="hit-path">{h.path.startsWith(cwd) ? h.path.slice(cwd.length).replace(/^\//, '') : h.path}{h.line ? `:${h.line}` : ''}</span>
                      {h.text && <span className="hit-text">{h.text}</span>}
                    </button>
                  </li>
                ))}
              </ul>
            </>
          )}
        </div>
      )}

      <div className="files-list" role="listbox" aria-multiselectable="true" aria-label={`Files in ${cwd}`} tabIndex={0}>
        <div className="files-head" role="row">
          <span role="columnheader" aria-sort={sort.key === 'name' ? (sort.asc ? 'ascending' : 'descending') : 'none'}>
            <button className="sorter" onClick={() => by('name')}>
              Name {arrow('name')}
            </button>
          </span>
          <span role="columnheader" aria-sort={sort.key === 'size' ? (sort.asc ? 'ascending' : 'descending') : 'none'}>
            <Splitter value={layout.cols.size} onChange={(px) => onLayout((l) => ({ ...l, cols: { ...l.cols, size: clamp(px, LIMITS.size) } }))} dir={-1} label="Resize the Size column" reset={DEFAULT.cols.size} />
            <button className="sorter" data-r onClick={() => by('size')}>
              {arrow('size')} Size
            </button>
          </span>
          <span role="columnheader" aria-sort={sort.key === 'mtime' ? (sort.asc ? 'ascending' : 'descending') : 'none'}>
            <Splitter value={layout.cols.modified} onChange={(px) => onLayout((l) => ({ ...l, cols: { ...l.cols, modified: clamp(px, LIMITS.modified) } }))} dir={-1} label="Resize the Modified column" reset={DEFAULT.cols.modified} />
            <button className="sorter" onClick={() => by('mtime')}>
              Modified {arrow('mtime')}
            </button>
          </span>
        </div>
        {editing?.isNew && <Rename key="new" initial="" kind={editing.isNew} onDone={commitEdit} onCancel={() => setEditing(null)} />}
        {entries.map((f) =>
          editing?.path === f.path && !editing.isNew ? (
            <Rename key={f.path} initial={f.name} kind={f.dir ? 'dir' : 'file'} onDone={commitEdit} onCancel={() => setEditing(null)} />
          ) : (
            <div
              key={f.path}
              role="option"
              aria-selected={sel.includes(f.path)}
              className="frow"
              data-path={f.path}
              data-dir-path={f.dir ? f.path : undefined}
              onClick={(e) => click(e, f)}
              onDoubleClick={() => open(f)}
              title={`${f.path}  ${f.mode}`}
            >
              <span className="fname">
                {f.dir ? <Folder size={16} strokeWidth={1.75} className="ico dir" /> : TEXTY.test(f.name) ? <FileText size={16} strokeWidth={1.75} className="ico" /> : <FileIcon size={16} strokeWidth={1.75} className="ico" />}
                <span>{f.name}</span>
                {f.link && <Link2 size={12} strokeWidth={2} className="lnk" aria-label="link" />}
              </span>
              <span className="fnum">{f.dir ? '' : humanSize(f.size)}</span>
              <span className="fdate">{shortDate(f.mtime)}</span>
            </div>
          ),
        )}
        {list && !entries.length && !editing?.isNew && <div className="files-none">This folder is empty. Drop files here to upload them.</div>}
      </div>

      {error && (
        <div className="notice bad" role="alert">
          <span>{error}</span>
          <button className="btn icon" onClick={() => setError('')} aria-label="Dismiss">
            <X size={14} strokeWidth={2} />
          </button>
        </div>
      )}
      <div className="files-status" aria-live="polite">
        <span>{msg || (chosen.length ? `${chosen.length} selected` : `${entries.length} item${entries.length === 1 ? '' : 's'}`)}</span>
        {chosen.length === 1 && !chosen[0].dir && <span>{chosen[0].mode}</span>}
      </div>

      {transfers.length > 0 && (
        <>
        <Splitter vertical value={layout.xferH} onChange={(px) => onLayout((l) => ({ ...l, xferH: clamp(px, LIMITS.xferH) }))} dir={-1} label="Resize the transfers list" reset={DEFAULT.xferH} />
        <section className="xfers" aria-label="Transfers" style={{ maxHeight: layout.xferH }}>
          <header>
            <strong>Transfers{active.length ? ` · ${active.length} running` : ''}</strong>
            <button className="btn" onClick={() => backend.files.clear()} disabled={transfers.length === active.length}>
              Clear finished
            </button>
          </header>
          <ul>
            {transfers.map((t) => (
              <li key={t.id} data-s={t.state}>
                <span className="xname" title={t.name}>
                  {t.up ? <Upload size={13} strokeWidth={2} /> : <Download size={13} strokeWidth={2} />} {t.name}
                </span>
                <span className="xstate">{t.state === 'running' || t.state === 'queued' ? (t.total ? `${Math.round((100 * t.done) / t.total)}%` : '…') : t.state === 'failed' ? 'Failed' : t.state === 'cancelled' ? 'Cancelled' : 'Done'}</span>
                <progress value={t.total ? t.done : undefined} max={t.total || undefined} aria-label={`${t.name} progress`} />
                <span className="xact">
                  {(t.state === 'running' || t.state === 'queued') && (
                    <button className="btn" onClick={() => backend.files.cancel(t.id)}>
                      Cancel
                    </button>
                  )}
                  {(t.state === 'failed' || t.state === 'cancelled') && (
                    <button className="btn" onClick={() => backend.files.retry(t.id)}>
                      Retry
                    </button>
                  )}
                </span>
                {t.error && <span className="xerr">{t.error}</span>}
              </li>
            ))}
          </ul>
        </section>
        </>
      )}

      {over && (
        <div className="dropveil" aria-hidden="true">
          <Upload size={28} strokeWidth={1.5} />
          <strong>Drop to upload to {basename(cwd) || '/'}</strong>
          <span>Drop on a folder to put them inside it</span>
        </div>
      )}
      {ask && (
        <div className="dimmer" role="alertdialog" aria-modal="true" aria-label="Files already exist">
          <div className="dialog">
            <header>sshdeck</header>
            <div className="body">
              <h2 className="name">{ask.res.conflicts!.length === 1 ? `“${ask.res.conflicts![0]}” already exists` : `${ask.res.conflicts!.length} items already exist`}</h2>
              <p>{ask.res.conflicts!.slice(0, 4).join(', ')}{ask.res.conflicts!.length > 4 ? ` and ${ask.res.conflicts!.length - 4} more` : ''} in {ask.kind === 'up' ? ask.dir : 'the folder you chose'}. Replacing a folder merges into it.</p>
              <div className="actions" style={{ justifyContent: 'flex-end' }}>
                <button className="btn solid" autoFocus onClick={() => void resolve('keep')}>
                  Keep both
                </button>
                <button className="btn" onClick={() => void resolve('skip')}>
                  Skip
                </button>
                <button className="btn danger" onClick={() => void resolve('replace')}>
                  Replace
                </button>
                <button className="btn" onClick={() => void resolve('cancel')}>
                  Cancel
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
      {del && (
        <Confirm
          title={del.length === 1 ? `Delete “${basename(del[0])}”?` : `Delete ${del.length} items?`}
          body={`${serverName}: this removes ${del.length === 1 ? 'it' : 'them'} from the server, folders with everything inside. There is no recycle bin.`}
          confirmLabel="Delete"
          onCancel={() => setDel(null)}
          onConfirm={() => void remove()}
        />
      )}
    </div>
  )
}

function Rename({ initial, kind, onDone, onCancel }: { initial: string; kind: 'dir' | 'file'; onDone: (n: string) => void; onCancel: () => void }) {
  const [v, setV] = useState(initial)
  const input = useRef<HTMLInputElement>(null)
  const done = useRef(false) // Enter and the blur it causes must not both commit
  const finish = (name: string | null) => {
    if (done.current) return
    done.current = true
    if (name === null) onCancel()
    else onDone(name)
  }
  useEffect(() => {
    input.current?.focus()
    const dot = initial.lastIndexOf('.')
    input.current?.setSelectionRange(0, dot > 0 ? dot : initial.length)
  }, [initial])
  return (
    <div className="frow editing">
      <span className="fname">
        {kind === 'dir' ? <Folder size={16} strokeWidth={1.75} className="ico dir" /> : <FileIcon size={16} strokeWidth={1.75} className="ico" />}
        <input
          ref={input}
          value={v}
          aria-label={initial ? 'New name' : kind === 'dir' ? 'Name of the new folder' : 'Name of the new file'}
          placeholder={kind === 'dir' ? 'New folder' : 'new-file.txt'}
          onChange={(e) => setV(e.target.value)}
          onKeyDown={(e) => {
            e.stopPropagation()
            if (e.key === 'Enter') finish(v)
            else if (e.key === 'Escape') finish(null)
          }}
          onBlur={() => finish(v.trim() && v !== initial ? v : null)}
        />
      </span>
    </div>
  )
}
