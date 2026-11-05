import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { Terminal } from '@xterm/xterm'
import { useEffect, useRef, useState } from 'react'
import type { Backend } from '../api'
import type { ContainerTab, Server } from '../types'

interface Props {
  server: Server
  tab: number
  container?: ContainerTab
  backend: Backend
  visible: boolean
}

/** One xterm instance per tab. Tabs stay mounted so switching is instant. */
export function Term({ server, tab, container, backend, visible }: Props) {
  const host = useRef<HTMLDivElement>(null)
  const term = useRef<Terminal | null>(null)
  const fit = useRef<FitAddon | null>(null)
  const [ended, setEnded] = useState(false)
  const restart = useRef<() => void>(() => {})
  const search = useRef<SearchAddon | null>(null)
  const findBox = useRef<HTMLInputElement>(null)
  const [finding, setFinding] = useState(false)
  const [query, setQuery] = useState('')
  const endedRef = useRef(false)
  endedRef.current = ended

  useEffect(() => {
    const plate = () => getComputedStyle(document.documentElement).getPropertyValue('--plate').trim() || '#111418'
    const t = new Terminal({
      fontFamily: "'JetBrains Mono Variable', monospace",
      fontSize: 14,
      cursorBlink: true,
      theme: {
        background: plate(),
        foreground: '#c0c0c0',
        cursor: '#c0c0c0',
        selectionBackground: '#c0c0c0',
        selectionForeground: '#000000',
      },
    })
    const f = new FitAddon()
    t.loadAddon(f)
    const sa = new SearchAddon()
    t.loadAddon(sa)
    search.current = sa
    // Ctrl+Shift+F opens the find bar instead of reaching the shell.
    t.attachCustomKeyEventHandler((e) => {
      if (e.type === 'keydown' && e.ctrlKey && e.shiftKey && e.key.toLowerCase() === 'f') {
        setFinding(true)
        return false
      }
      return true
    })
    t.open(host.current!)
    term.current = t
    const retheme = () => (t.options.theme = { ...t.options.theme, background: plate() })
    window.addEventListener('sshdeck:theme', retheme)
    fit.current = f
    let live = true
    let detach = () => {}
    f.fit()
    // Opens (or re-opens) this tab's shell. After the link drops, the same call makes a fresh SSH session.
    const begin = () => {
      setEnded(false)
      detach()
      detach = () => {}
      ;(container ? backend.openContainer(server.id, tab, container.container, container.user, container.sudo) : Promise.resolve())
        .then(() =>
          backend.attach(
            server.id,
            tab,
            { cols: t.cols, rows: t.rows },
            (d) => live && t.write(d),
            () => {
              if (!live) return
              t.write('\r\n\x1b[2m[session closed]\x1b[0m\r\n')
              setEnded(true)
            },
          ),
        )
        .then((off) => (live ? (detach = off) : off()))
        .catch((e) => {
          if (!live) return
          t.write(`\r\n\x1b[31m${e instanceof Error ? e.message : String(e)}\x1b[0m\r\n`)
          setEnded(true)
        })
    }
    restart.current = begin
    begin()
    // The Go side redialled the dropped link: a shell that ended with it starts again by itself.
    const offLink = backend.onLink(server.id, (l) => l.state === 'live' && endedRef.current && live && begin())
    t.onData((d) => backend.write(server.id, tab, d))
    t.onResize(({ cols, rows }) => backend.resize(server.id, tab, cols, rows))
    const ro = new ResizeObserver(() => host.current?.offsetParent && f.fit())
    ro.observe(host.current!)
    return () => {
      live = false
      offLink()
      detach()
      backend.closeShell(server.id, tab)
      ro.disconnect()
      window.removeEventListener('sshdeck:theme', retheme)
      t.dispose()
    }
    // server.id, not server: editing a server's details must not tear down its open shell.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [server.id, tab, backend])

  useEffect(() => {
    if (visible) {
      fit.current?.fit()
      term.current?.focus()
    }
  }, [visible])

  useEffect(() => {
    if (finding) findBox.current?.select()
  }, [finding])

  const closeFind = () => {
    setFinding(false)
    search.current?.clearDecorations()
    term.current?.focus()
  }
  const find = (back: boolean, q = query) => {
    if (!q) return
    const opts = { decorations: { matchOverviewRuler: '#e8a317', activeMatchColorOverviewRuler: '#e8a317', matchBackground: '#7a5c00', activeMatchBackground: '#e8a317' } }
    if (back) search.current?.findPrevious(q, opts)
    else search.current?.findNext(q, { ...opts, incremental: true })
  }

  return (
    <div className="term" style={{ display: visible ? 'block' : 'none' }}>
      {finding && (
        <div className="findbar" role="search">
          <input
            ref={findBox}
            autoFocus
            value={query}
            aria-label="Find in terminal"
            placeholder="Find in terminal"
            onChange={(e) => (setQuery(e.target.value), find(false, e.target.value))}
            onKeyDown={(e) => {
              if (e.key === 'Enter') find(e.shiftKey)
              else if (e.key === 'Escape') closeFind()
            }}
          />
          <button className="btn" onClick={() => find(true)} aria-label="Previous match">↑</button>
          <button className="btn" onClick={() => find(false)} aria-label="Next match">↓</button>
          <button className="btn" onClick={closeFind} aria-label="Close find">✕</button>
        </div>
      )}
      <div ref={host} style={{ height: '100%' }} />
      {ended && (
        <button className="btn reconnect" onClick={() => restart.current()}>
          Reconnect
        </button>
      )}
    </div>
  )
}
