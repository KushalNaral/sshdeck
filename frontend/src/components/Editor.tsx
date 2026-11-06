import { History, Save, RotateCcw, WrapText } from 'lucide-react'
import type * as Monaco from 'monaco-editor'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { Backend } from '../api'
import { languageFor, loadMonaco, wantDark } from '../lib/monaco'
import type { Server } from '../types'
import { Confirm } from './Confirm'
import { DiffView } from './DiffView'
import { HistoryDrawer, type DiffPick } from './HistoryDrawer'

interface Props {
  /** Where the file is: the server id, or "serverId@container". */
  target: string
  server: Server
  /** Whether the server has a live connection (needed to restore). */
  live: boolean
  /** Counts up each time something asks for this file's history to open. */
  historyAsk: number
  onCopy: (mode: '' | 'on' | 'off') => void
  path: string
  backend: Backend
  visible: boolean
  onDirty: (dirty: boolean) => void
}

type Conflict = 'changed' | null

/** One file open in Monaco. It stays mounted (hidden) while another tab is in front, so edits and undo history survive. */
export function Editor({ target, server, live, historyAsk, onCopy, path, backend, visible, onDirty }: Props) {
  const serverId = server.id
  const host = useRef<HTMLDivElement>(null)
  const ed = useRef<Monaco.editor.IStandaloneCodeEditor | null>(null)
  const mon = useRef<typeof Monaco | null>(null)
  const version = useRef('')
  const saved = useRef('')
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [error, setError] = useState('')
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)
  const [conflict, setConflict] = useState<Conflict>(null)
  const [wrap, setWrap] = useState(false)
  const [pos, setPos] = useState({ line: 1, col: 1 })
  const [lang, setLang] = useState('plaintext')
  const [mode, setMode] = useState('')
  const [note, setNote] = useState('')
  const [drawer, setDrawer] = useState(false)
  const [rev, setRev] = useState(0)
  const [diff, setDiff] = useState<DiffPick | null>(null)
  const [noHist, setNoHist] = useState(false)
  const [ask, setAsk] = useState(false)
  useEffect(() => void (historyAsk > 0 && setDrawer(true)), [historyAsk])

  const markDirty = useCallback(
    (d: boolean) => {
      setDirty(d)
      onDirty(d)
    },
    [onDirty],
  )

  const save = useCallback(
    async (force = false) => {
      const e = ed.current
      if (!e || busy) return
      setBusy(true)
      setNote('')
      try {
        const text = e.getValue()
        version.current = await backend.files.write(serverId, path, text, force ? '' : version.current)
        saved.current = text
        setConflict(null)
        markDirty(false)
        setNote('Saved')
        setRev((r) => r + 1)
      } catch (err) {
        const m = err instanceof Error ? err.message : String(err)
        if (m.includes('changed on the server')) setConflict('changed')
        else setNote(`Not saved: ${m}`)
      }
      setBusy(false)
    },
    [backend, busy, markDirty, path, serverId],
  )
  const saveRef = useRef(save)
  saveRef.current = save

  const load = useCallback(async () => {
    setState('loading')
    try {
      const [m, f] = await Promise.all([loadMonaco(), backend.files.read(serverId, path)])
      mon.current = m
      version.current = f.version
      saved.current = f.content
      setMode(f.mode)
      setNoHist(!!f.noHistory)
      setRev((r) => r + 1)
      setLang(languageFor(m, path))
      if (!ed.current && host.current) {
        const model = m.editor.createModel(f.content, languageFor(m, path), m.Uri.parse(`sshdeck://${serverId.replace('@', '.')}${path}`))
        ed.current = m.editor.create(host.current, {
          model,
          theme: wantDark() ? 'sshdeck-dark' : 'sshdeck-light',
          automaticLayout: true,
          fontFamily: "'JetBrains Mono Variable', monospace",
          fontSize: 13,
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          renderWhitespace: 'selection',
          smoothScrolling: true,
          padding: { top: 6 },
        })
        ed.current.addCommand(m.KeyMod.CtrlCmd | m.KeyCode.KeyS, () => void saveRef.current())
        ed.current.onDidChangeCursorPosition((ev) => setPos({ line: ev.position.lineNumber, col: ev.position.column }))
        ed.current.onDidChangeModelContent(() => markDirty(ed.current!.getValue() !== saved.current))
      } else {
        ed.current?.setValue(f.content)
        markDirty(false)
      }
      setConflict(null)
      setState('ready')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setState('error')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [backend, path, serverId])

  useEffect(() => {
    void load()
    return () => {
      const model = ed.current?.getModel()
      ed.current?.dispose()
      model?.dispose()
      ed.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, path])

  // Follow the app theme.
  useEffect(() => {
    const apply = () => mon.current?.editor.setTheme(wantDark() ? 'sshdeck-dark' : 'sshdeck-light')
    const mq = matchMedia('(prefers-color-scheme: dark)')
    window.addEventListener('sshdeck:theme', apply)
    mq.addEventListener('change', apply)
    return () => (window.removeEventListener('sshdeck:theme', apply), mq.removeEventListener('change', apply))
  }, [])

  useEffect(() => {
    if (visible && state === 'ready') ed.current?.focus()
  }, [visible, state])
  useEffect(() => void ed.current?.updateOptions({ wordWrap: wrap ? 'on' : 'off' }), [wrap])

  const name = path.slice(path.lastIndexOf('/') + 1)
  return (
    <section className="editor" hidden={!visible} aria-label={`Editing ${name}`}>
      <div className="edbar">
        <button className="btn" onClick={() => void save()} disabled={!dirty || busy || state !== 'ready'} title="Save (Ctrl+S)">
          <Save size={14} strokeWidth={2} /> Save
        </button>
        <button
          className="btn"
          onClick={() => void load()}
          disabled={state === 'loading' || busy}
          title="Throw away your changes and read the file from the server again"
        >
          <RotateCcw size={14} strokeWidth={2} /> Reload
        </button>
        <button className="btn" aria-pressed={drawer} onClick={() => setDrawer((d) => !d)} title="Earlier versions of this file: who changed it and when">
          <History size={14} strokeWidth={2} /> History
        </button>
        <button className="btn" aria-pressed={wrap} onClick={() => setWrap((w) => !w)} title="Wrap long lines">
          <WrapText size={14} strokeWidth={2} /> Wrap
        </button>
        <span className="edpath" title={path}>
          {path}
        </span>
      </div>
      {conflict && (
        <div className="notice warn" role="alert">
          <strong>{name} changed on the server after you opened it.</strong>
          <span>Saving now would overwrite their version.</span>
          <button className="btn" onClick={() => void save(true)}>
            Overwrite
          </button>
          <button className="btn" onClick={() => void load()}>
            Reload theirs
          </button>
          <button className="btn" onClick={() => setConflict(null)}>
            Keep editing
          </button>
        </div>
      )}
      <div className="edbody">
        <div className="edmain">
          <div className="edhost" ref={host} />
          {diff && (
            <div className="diffwrap">
              <div className="diffbar">
                <span>
                  <strong>{diff.a.label}</strong> → <strong>{diff.b.label}</strong>
                </span>
                {diff.restore && (
                  <button className="btn" disabled={!live} onClick={() => setAsk(true)} title={live ? 'Write the older version back to the server' : 'Connect to restore'}>
                    Restore the older version
                  </button>
                )}
                <button className="btn" onClick={() => setDiff(null)}>
                  Close diff
                </button>
              </div>
              <DiffView original={diff.a.text} modified={diff.b.text} language={lang} />
            </div>
          )}
        </div>
        {drawer && (
          <HistoryDrawer server={server} target={target} path={path} backend={backend} rev={rev} current={() => ed.current?.getValue() ?? ''} live={live} noHistory={noHist} onPick={setDiff} onCopy={onCopy} onClose={() => (setDrawer(false), setDiff(null))} />
        )}
      </div>
      {state === 'loading' && <div className="empty"><strong>Opening {name}…</strong></div>}
      {state === 'error' && (
        <div className="empty">
          <strong>{name} could not be opened</strong>
          <span>{error}</span>
          <button className="btn" onClick={() => void load()}>
            Try again
          </button>
        </div>
      )}
      {ask && diff?.restore && (
        <Confirm
          title={`Restore ${name} to this version?`}
          body={`${diff.a.label}. The file on ${server.name} is replaced now, and what is there today stays in history, so you can undo this.`}
          confirmLabel="Restore"
          onCancel={() => setAsk(false)}
          onConfirm={() => {
            const v = diff.restore!
            setAsk(false)
            void backend.history
              .restore(serverId, path, v.id)
              .then(() => (setDiff(null), load()))
              .catch((e) => setNote(`Not restored: ${e instanceof Error ? e.message : String(e)}`))
          }}
        />
      )}
      <div className="edstatus" aria-live="polite">
        <span>{dirty ? 'Modified' : (note || 'Saved')}</span>
        <span>{lang}</span>
        <span>{mode}</span>
        <span>UTF-8</span>
        <span>
          Ln {pos.line}, Col {pos.col}
        </span>
      </div>
    </section>
  )
}
