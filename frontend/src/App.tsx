import { useCallback, useEffect, useRef, useState } from 'react'
import { ConnectError } from './api'
import { backend } from './backend'
import { Admin } from './components/Admin'
import { Connection } from './components/Connection'
import { Mark } from './components/Mark'
import { Analysis } from './components/Analysis'
import { ConnectBar } from './components/ConnectBar'
import { PatchBoard } from './components/PatchBoard'
import { Palette } from './components/Palette'
import { Rack } from './components/Rack'
import { TerminalPane } from './components/TerminalPane'
import { configStore, messageOf, type Config } from './lib/config'
import { vpnOf } from './lib/status'
import type { Analysis as AnalysisData, ContainerTab, Mode, Server, Session, Vpn } from './types'

export default function App() {
  const [mode, setMode] = useState<Mode>('firefight')
  const [config, setConfig] = useState<Config>({ servers: [], vpns: [] })
  const [loaded, setLoaded] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [selected, setSelected] = useState('')
  const [sessions, setSessions] = useState<Record<string, Session>>({})
  const [analysis, setAnalysis] = useState<Record<string, AnalysisData>>({})
  const [scanning, setScanning] = useState<string | null>(null)
  const [scanErrors, setScanErrors] = useState<Record<string, string>>({})
  const [palette, setPalette] = useState(false)
  const [light, setLight] = useState(0)

  const servers = config.servers
  const server = servers.find((s) => s.id === selected) ?? servers[0]
  const vpns = config.vpns
  const latest = useRef(config)
  latest.current = config

  useEffect(() => {
    configStore
      .load()
      .then((c) => {
        setConfig(c)
        setSelected(c.servers[0]?.id ?? '')
        if (!c.servers.length) setMode('admin')
        setLoaded(true)
      })
      .catch((e) => setLoadError(messageOf(e)))
  }, [])

  // shortcut: a connect still in flight when its session is dropped is ignored, not cancelled; cancel via the Go side later.
  const patch = useCallback((id: string, fn: (s: Session) => Session) => setSessions((p) => (p[id] ? { ...p, [id]: fn(p[id]) } : p)), [])

  const connect = useCallback(
    async (id: string) => {
      const sv = latest.current.servers.find((s) => s.id === id)!
      const vpn = latest.current.vpns.find((v) => v.id === sv.vpnId)
      setSessions((p) => ({ ...p, [id]: { serverId: id, state: 'connecting', steps: [], tabs: [], activeTab: 0, nextTab: 1 } }))
      setLight((n) => n + 1)
      try {
        await backend.connect(sv, vpn, (steps) => patch(id, (s) => ({ ...s, steps })))
        patch(id, (s) => ({ ...s, state: 'live', since: Date.now(), tabs: [1], activeTab: 1, nextTab: 2 }))
      } catch (e) {
        const err = e instanceof ConnectError ? e : new ConnectError('The connection failed.', 'Try again; if it keeps failing, check the credentials.')
        patch(id, (s) => ({ ...s, state: 'failed', error: { problem: err.problem, recovery: err.recovery } }))
      }
      setLight((n) => n + 1)
    },
    [patch],
  )

  const disconnect = (id: string) => {
    backend.disconnect(id)
    setSessions((p) => {
      const { [id]: _, ...rest } = p
      return rest
    })
  }

  const addTab = useCallback(
    (id: string, info?: ContainerTab) =>
      patch(id, (s) => ({ ...s, tabs: [...s.tabs, s.nextTab], activeTab: s.nextTab, nextTab: s.nextTab + 1, tabInfo: info ? { ...s.tabInfo, [s.nextTab]: info } : s.tabInfo })),
    [patch],
  )
  const closeTab = (id: string, tab: number) =>
    patch(id, (s) => {
      const tabs = s.tabs.filter((t) => t !== tab)
      return { ...s, tabs, activeTab: s.activeTab === tab ? tabs[tabs.length - 1] : s.activeTab }
    })

  const scan = useCallback(async (id: string) => {
    setScanning(id)
    setScanErrors((p) => ({ ...p, [id]: '' }))
    try {
      const data = await backend.analyze(latest.current.servers.find((s) => s.id === id)!)
      setAnalysis((p) => ({ ...p, [id]: data }))
    } catch (e) {
      setScanErrors((p) => ({ ...p, [id]: messageOf(e) }))
    }
    setScanning((cur) => (cur === id ? null : cur))
  }, [])

  // Scan on its own only for a server with an open shell: a scan logs in, and the others may be behind a VPN that is down.
  const live = sessions[selected]?.state === 'live'
  useEffect(() => {
    if (live && !analysis[selected] && !scanErrors[selected] && scanning !== selected) void scan(selected)
  }, [live, selected, analysis, scanErrors, scanning, scan])

  // Each returns an error message for the form to show, or null on success.
  const attempt = async (fn: () => Promise<void>) => {
    try {
      await fn()
      return null
    } catch (e) {
      return messageOf(e)
    }
  }
  const upsert = <T extends { id: string }>(list: T[], x: T) => (list.some((y) => y.id === x.id) ? list.map((y) => (y.id === x.id ? x : y)) : [...list, x])

  const saveServer = (s: Server, pw: string) =>
    attempt(async () => {
      const saved = await configStore.saveServer(s, pw)
      setConfig((c) => ({ ...c, servers: upsert(c.servers, saved) }))
      setSelected((cur) => cur || saved.id)
      setAnalysis((p) => {
        const { [saved.id]: _, ...rest } = p
        return rest
      })
    })
  const deleteServer = (id: string) =>
    attempt(async () => {
      await configStore.deleteServer(id)
      disconnect(id)
      setConfig((c) => ({ ...c, servers: c.servers.filter((x) => x.id !== id) }))
      setSelected((cur) => (cur === id ? (latest.current.servers.find((x) => x.id !== id)?.id ?? '') : cur))
    })
  const saveVpn = (v: Vpn, pw: string) =>
    attempt(async () => {
      const saved = await configStore.saveVpn(v, pw)
      setConfig((c) => ({ ...c, vpns: upsert(c.vpns, saved) }))
    })
  const deleteVpn = (id: string) =>
    attempt(async () => {
      await configStore.deleteVpn(id)
      setConfig((c) => ({ ...c, vpns: c.vpns.filter((x) => x.id !== id) }))
    })

  // The patch stage is saved with the server, so it survives restarts.
  const setStage = (id: string, stage: number) => {
    const sv = latest.current.servers.find((s) => s.id === id)
    if (sv) void saveServer({ ...sv, stage }, '')
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.ctrlKey) return
      if (e.key === 'k') (e.preventDefault(), setPalette(true))
      else if (e.key === 't' && sessions[selected]?.state === 'live') (e.preventDefault(), addTab(selected))
      else if (e.key === '1') (e.preventDefault(), setMode('firefight'))
      else if (e.key === '2') (e.preventDefault(), setMode('patch'))
      else if (e.key === '3') (e.preventDefault(), setMode('admin'))
    }
    // Capture phase: xterm stops key events before they bubble, so shortcuts must be caught first.
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [selected, sessions, addTab])

  const sess = sessions[selected]

  if (!loaded) return <div className="app">{loadError ? <p className="bad" role="alert" style={{ padding: 24 }}>Could not load your servers: {loadError}</p> : null}</div>

  return (
    <div className="app">
      {light > 0 && <div key={light} className="light" aria-hidden />}
      <header className="top">
        <h1 className="brand">sshdeck</h1>
        <div className="mode" role="group" aria-label="Job">
          <button aria-pressed={mode === 'firefight'} onClick={() => setMode('firefight')}>
            <Mark kind={mode === 'firefight' ? 'filled' : 'hollow'} />
            <span>A · Firefight</span>
          </button>
          <button aria-pressed={mode === 'patch'} onClick={() => setMode('patch')}>
            <Mark kind={mode === 'patch' ? 'filled' : 'hollow'} />
            <span>B · Patch</span>
          </button>
          <button aria-pressed={mode === 'admin'} onClick={() => setMode('admin')}>
            <Mark kind={mode === 'admin' ? 'filled' : 'hollow'} />
            <span>C · Admin</span>
          </button>
        </div>
        <div className="hints">
          <span>
            <kbd>Ctrl K</kbd> Find
          </span>
          <span>
            <kbd>Ctrl T</kbd> New shell
          </span>
          <span>
            <kbd>Ctrl 1</kbd> <kbd>2</kbd> <kbd>3</kbd> Job
          </span>
        </div>
      </header>

      {server && (
      <main className="stage" hidden={mode !== 'firefight'}>
        <Rack
          servers={servers}
          vpns={vpns}
          sessions={sessions}
          selected={selected}
          onSelect={setSelected}
          onConnect={(id) => !sessions[id] && void connect(id)}
        />
        <div className="center">
          <ConnectBar
            server={server}
            vpn={vpnOf(server, vpns)}
            session={sess}
            onConnect={() => void connect(selected)}
            onDisconnect={() => disconnect(selected)}
            onManualDone={() => backend.confirmManualVpn(selected)}
          />
          <TerminalPane
            servers={config.servers}
            sessions={sessions}
            selected={selected}
            backend={backend}
            onTab={(id, tab) => patch(id, (s) => ({ ...s, activeTab: tab }))}
            onAddTab={addTab}
            onCloseTab={closeTab}
          />
        </div>
        <Analysis server={server} data={analysis[selected]} scanning={scanning === selected} error={scanErrors[selected]} connected={live} onExec={(c) => addTab(selected, c)} onRescan={() => void scan(selected)} connection={<Connection server={server} vpns={vpns} session={sess} />} />
      </main>
      )}

      {mode === 'patch' && (
        <main className="stage">
          <PatchBoard
            servers={servers}
            vpns={vpns}
            onStage={setStage}
            onOpen={(id) => {
              setSelected(id)
              setMode('firefight')
            }}
          />
        </main>
      )}

      {mode === 'admin' && (
        <main className="stage">
          <Admin servers={servers} vpns={vpns} onSaveServer={saveServer} onDeleteServer={deleteServer} onSaveVpn={saveVpn} onDeleteVpn={deleteVpn} />
        </main>
      )}

      {palette && (
        <Palette
          servers={servers}
          vpns={vpns}
          sessions={sessions}
          onClose={() => setPalette(false)}
          onPick={(id, go) => {
            setPalette(false)
            setMode('firefight')
            setSelected(id)
            if (go && !sessions[id]) void connect(id)
          }}
        />
      )}
    </div>
  )
}
