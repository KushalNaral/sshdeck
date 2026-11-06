import { Save, RotateCcw, WrapText } from 'lucide-react'
import type * as Monaco from 'monaco-editor'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { Backend } from '../api'
import { languageFor, loadMonaco, wantDark } from '../lib/monaco'

interface Props {
  serverId: string
  path: string
  backend: Backend
  visible: boolean
  onDirty: (dirty: boolean) => void
}

type Conflict = 'changed' | null

/** One file open in Monaco. It stays mounted (hidden) while another tab is in front, so edits and undo history survive. */
export function Editor({ serverId, path, backend, visible, onDirty }: Props) {
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
      setLang(languageFor(m, path))
      if (!ed.current && host.current) {
        const model = m.editor.createModel(f.content, languageFor(m, path), m.Uri.parse(`sshdeck://${serverId}${path}`))
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
      <div className="edhost" ref={host} />
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
