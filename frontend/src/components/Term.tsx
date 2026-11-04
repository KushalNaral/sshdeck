import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import { useEffect, useRef } from 'react'
import type { Backend } from '../api'
import type { Server } from '../types'

interface Props {
  server: Server
  backend: Backend
  visible: boolean
}

/** One xterm instance per tab. Tabs stay mounted so switching is instant. */
export function Term({ server, backend, visible }: Props) {
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
    const prompt = `\x1b[32m${server.user}@${server.name.toLowerCase().replace(/\s+/g, '-')}\x1b[0m:~$ `
    t.write(`Last login: today from your desk\r\n(demo shell: try uptime, php -v, df -h, free -m, docker ps, systemctl --failed)\r\n${prompt}`)
    let line = ''
    t.onData((d) => {
      if (d === '\r') {
        t.write('\r\n' + backend.exec(server, line) + prompt)
        line = ''
      } else if (d === '\x7f') {
        if (line) {
          line = line.slice(0, -1)
          t.write('\b \b')
        }
      } else if (d >= ' ') {
        line += d
        t.write(d)
      }
    })
    const ro = new ResizeObserver(() => host.current?.offsetParent && f.fit())
    ro.observe(host.current!)
    return () => {
      ro.disconnect()
      t.dispose()
    }
  }, [server, backend])

  useEffect(() => {
    if (visible) {
      fit.current?.fit()
      term.current?.focus()
    }
  }, [visible])

  return <div ref={host} className="term" style={{ display: visible ? 'block' : 'none' }} />
}
