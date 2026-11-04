import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import { useEffect, useRef } from 'react'
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

  useEffect(() => {
    const css = getComputedStyle(document.documentElement)
    const t = new Terminal({
      fontFamily: "'JetBrains Mono Variable', monospace",
      fontSize: 14,
      cursorBlink: true,
      theme: {
        background: css.getPropertyValue('--plate').trim() || '#111418',
        foreground: '#f2efe4',
        cursor: '#f6d27a',
        selectionBackground: '#d9a21b',
        selectionForeground: '#111418',
      },
    })
    const f = new FitAddon()
    t.loadAddon(f)
    t.open(host.current!)
    term.current = t
    fit.current = f
    let live = true
    let detach = () => {}
    f.fit()
    ;(container ? backend.openContainer(server.id, tab, container.container, container.user, container.sudo) : Promise.resolve())
      .then(() => backend.attach(
        server.id,
        tab,
        { cols: t.cols, rows: t.rows },
        (d) => live && t.write(d),
        () => live && t.write('\r\n\x1b[2m[session closed]\x1b[0m\r\n'),
      ))
      .then((off) => (live ? (detach = off) : off()))
      .catch((e) => live && t.write(`\r\n\x1b[31m${e instanceof Error ? e.message : String(e)}\x1b[0m\r\n`))
    t.onData((d) => backend.write(server.id, tab, d))
    t.onResize(({ cols, rows }) => backend.resize(server.id, tab, cols, rows))
    const ro = new ResizeObserver(() => host.current?.offsetParent && f.fit())
    ro.observe(host.current!)
    return () => {
      live = false
      detach()
      backend.closeShell(server.id, tab)
      ro.disconnect()
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

  return <div ref={host} className="term" style={{ display: visible ? 'block' : 'none' }} />
}
