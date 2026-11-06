import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ConnectError } from './api'
import { backend } from './backend'
import { Admin } from './components/Admin'
import { LockScreen } from './components/LockScreen'
import { Confirm } from './components/Confirm'
import { Connection } from './components/Connection'
import { Mark } from './components/Mark'
import { Analysis } from './components/Analysis'
import { Logins } from './components/Logins'
import { updateApi, type UpdateInfo } from './lib/update'
import { ConnectBar } from './components/ConnectBar'
import { TasksBoard } from './components/TasksBoard'
import { Palette } from './components/Palette'
import { Rack } from './components/Rack'
import { TerminalPane, type Doc } from './components/TerminalPane'
import { FilesPanel } from './components/FilesPanel'
import { auditApi } from './lib/audit'
import { configStore, messageOf, type Config } from './lib/config'
import { useNow } from './lib/now'
import { vpnOf } from './lib/status'
import { triage } from './lib/triage'
import { lockApi, type LockState } from './lib/lock'
import { applyTheme, loadTheme, THEMES } from './lib/theme'
import type { Analysis as AnalysisData, ContainerTab, Login, Mode, Server, Session, Task, Vpn } from './types'

export default function App() {
  const [mode, setMode] = useState<Mode>('firefight')
  const [config, setConfig] = useState<Config>({ servers: [], vpns: [], tasks: [] })
  const [loaded, setLoaded] = useState(false)
  const [lock, setLock] = useState<LockState | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [selected, setSelected] = useState('')
  const [sessions, setSessions] = useState<Record<string, Session>>({})
  const [analysis, setAnalysis] = useState<Record<string, AnalysisData>>({})
  const [scanning, setScanning] = useState<string | null>(null)
  const [scanErrors, setScanErrors] = useState<Record<string, string>>({})
  const [palette, setPalette] = useState(false)
  const [newer, setNewer] = useState<UpdateInfo | null>(null) // the startup check found a newer release
  const [adminTab, setAdminTab] = useState<'system' | undefined>(undefined)
  useEffect(() => updateApi.onAvailable(setNewer), [])
  useEffect(() => {
    if (mode !== 'admin') setAdminTab(undefined)
  }, [mode])
  const [ask, setAsk] = useState<{ title: string; body: string; label: string; run: () => void; cancelled?: () => void } | null>(null)
  const [theme, setTheme] = useState(loadTheme)
  const cycleTheme = () => {
    const next = THEMES[(THEMES.indexOf(theme) + 1) % THEMES.length]
    applyTheme(next)
    setTheme(next)
  }

  const servers = config.servers
  const server = servers.find((s) => s.id === selected) ?? servers[0]
  const vpns = config.vpns
  const latest = useRef(config)
  const now = useNow(30_000)
  const attempts = useRef<Record<string, number>>({}) // connect attempt number per server, so a cancelled attempt's late result is ignored
  latest.current = config
  const sessionsRef = useRef(sessions)
  sessionsRef.current = sessions

  const lockRef = useRef(lock)
  lockRef.current = lock

  // The lock state comes first: nothing is loaded while the app is locked.
  useEffect(() => {
    lockApi.state().then(setLock).catch((e) => setLoadError(messageOf(e)))
    return lockApi.onLocked(() => {
      if (lockRef.current?.endOnLock) setSessions({}) // Go ended the shells and VPNs too
      setLock((s) => s && { ...s, locked: true })
    })
  }, [])

  useEffect(() => {
    if (!lock || lock.locked || loaded) return
    configStore
      .load()
      .then((c) => {
        setConfig(c)
        setSelected(c.servers[0]?.id ?? '')
        if (!c.servers.length) setMode('admin')
        setLoaded(true)
      })
      .catch((e) => setLoadError(messageOf(e)))
  }, [lock, loaded])

  const unlock = async (pass: string) => {
    try {
      await lockApi.unlock(pass)
      setLock(await lockApi.state())
      return null
    } catch (e) {
      return messageOf(e)
    }
  }

  // Tell Go the user is active (it owns the idle timer). Capture phase, because xterm stops key events.
  useEffect(() => {
    if (!lock?.enabled || lock.locked) return
    let last = 0
    const ping = () => {
      const t = Date.now()
      if (t - last > 15000) {
        last = t
        lockApi.touch()
      }
    }
    const evs = ['keydown', 'pointerdown', 'wheel'] as const
    evs.forEach((e) => window.addEventListener(e, ping, true))
    return () => evs.forEach((e) => window.removeEventListener(e, ping, true))
  }, [lock?.enabled, lock?.locked])

  // shortcut: a connect still in flight when its session is dropped is ignored, not cancelled; cancel via the Go side later.
  const patch = useCallback((id: string, fn: (s: Session) => Session) => setSessions((p) => (p[id] ? { ...p, [id]: fn(p[id]) } : p)), [])

  const connect = useCallback(
    async (id: string) => {
      const sv = latest.current.servers.find((s) => s.id === id)!
      const vpn = latest.current.vpns.find((v) => v.id === sv.vpnId)
      const n = (attempts.current[id] = (attempts.current[id] ?? 0) + 1)
      const current = () => attempts.current[id] === n
      setSessions((p) => ({ ...p, [id]: { serverId: id, state: 'connecting', steps: [], tabs: [], activeTab: 0, nextTab: 1 } }))
      try {
        await backend.connect(sv, vpn, (steps) => current() && patch(id, (s) => ({ ...s, steps })))
        if (!current()) return
        patch(id, (s) => ({ ...s, state: 'live', since: Date.now(), tabs: [1], activeTab: 1, nextTab: 2 }))
      } catch (e) {
        if (!current()) return
        const err = e instanceof ConnectError ? e : new ConnectError('The connection failed.', 'Try again; if it keeps failing, check the credentials.')
        patch(id, (s) => ({ ...s, state: 'failed', error: { problem: err.problem, recovery: err.recovery, details: err.details } }))
      }
    },
    [patch],
  )

  // A dropped link is redialled in Go; show how that is going on the server's session.
  const liveIds = Object.values(sessions).filter((s) => s.state === 'live').map((s) => s.serverId).sort().join(',')
  useEffect(() => {
    const offs = liveIds.split(',').filter(Boolean).map((id) => backend.onLink(id, (l) => patch(id, (s) => ({ ...s, link: l.state === 'live' ? undefined : l }))))
    return () => offs.forEach((off) => off())
  }, [liveIds, patch])

  const disconnect = (id: string) => {
    attempts.current[id] = (attempts.current[id] ?? 0) + 1
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
  // Production servers ask first. The confirmation is written to the audit trail, then the action runs.
  // Resolves when the connect has finished, or when the question was answered No.
  const requestConnect = (id: string): Promise<void> => {
    const sv = latest.current.servers.find((x) => x.id === id)
    if (!sv?.production) return connect(id)
    return new Promise((resolve) =>
      setAsk({
        title: `Connect to ${sv.name}?`,
        body: `${sv.name} is a production server. You will be signed in as ${sv.user}@${sv.host}.`,
        label: 'Connect to production',
        run: () => (auditApi.confirm(id, 'connect'), void connect(id).then(resolve)),
        cancelled: resolve,
      }),
    )
  }

  // Servers marked "connect on start" are connected once per launch, one after another so that password prompts
  // (a VPN asks for admin rights) never pile up. Production ones come last, each after its own confirmation.
  const started = useRef(false)
  useEffect(() => {
    if (!loaded || started.current || lock?.locked) return
    started.current = true
    const want = config.servers.filter((s) => s.autoConnect)
    void (async () => {
      for (const sv of [...want.filter((s) => !s.production), ...want.filter((s) => s.production)]) {
        if (!latest.current.servers.some((s) => s.id === sv.id) || sessionsRef.current[sv.id]) continue
        await requestConnect(sv.id)
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loaded, lock?.locked])
  const requestExec = (id: string, c: ContainerTab) => {
    const sv = latest.current.servers.find((x) => x.id === id)
    const asRoot = c.sudo || c.user === '' || c.user === 'root' || c.user === '0'
    if (!sv?.production || !asRoot) return addTab(id, c)
    setAsk({
      title: `Open a root shell in ${c.container}?`,
      body: `${sv.name} is a production server. This opens ${c.container} as ${c.sudo ? 'root with sudo' : c.user || 'its default user (usually root)'}.`,
      label: 'Open root shell',
      run: () => (auditApi.confirm(id, 'container'), addTab(id, c)),
    })
  }
  // Files open in the editor, per server. A tab shows a dot while it has unsaved changes.
  const [docs, setDocs] = useState<Record<string, Doc[]>>({})
  const [activeDoc, setActiveDoc] = useState<Record<string, string | null>>({})
  const [side, setSide] = useState<'analysis' | 'files'>('analysis')
  const openDoc = (id: string, path: string) => {
    setDocs((d) => ((d[id] ?? []).some((x) => x.path === path) ? d : { ...d, [id]: [...(d[id] ?? []), { path, dirty: false }] }))
    setActiveDoc((a) => ({ ...a, [id]: path }))
  }
  const docsRef = useRef(docs)
  docsRef.current = docs
  const dropDoc = (id: string, path: string) => {
    const rest = (docsRef.current[id] ?? []).filter((x) => x.path !== path)
    setDocs((d) => ({ ...d, [id]: rest }))
    setActiveDoc((a) => (a[id] === path ? { ...a, [id]: rest.length ? rest[rest.length - 1].path : null } : a))
  }
  const closeDoc = (id: string, path: string) => {
    if (!docs[id]?.find((x) => x.path === path)?.dirty) return dropDoc(id, path)
    setAsk({ title: `Close ${path.slice(path.lastIndexOf('/') + 1)} without saving?`, body: 'Your changes to this file will be lost.', label: 'Discard changes', run: () => dropDoc(id, path) })
  }
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
  const setLogins = (sid: string, f: (l: Login[]) => Login[]) => setConfig((c) => ({ ...c, servers: c.servers.map((s) => (s.id === sid ? { ...s, logins: f(s.logins ?? []) } : s)) }))
  const saveLogin = (sid: string, l: Login, pw: string) =>
    attempt(async () => {
      const saved = await configStore.saveLogin(sid, l, pw)
      setLogins(sid, (list) => upsert(list, saved))
    })
  const deleteLogin = (sid: string, id: string) =>
    attempt(async () => {
      await configStore.deleteLogin(sid, id)
      setLogins(sid, (list) => list.filter((x) => x.id !== id))
    })
  // Keeps the scan: saveServer drops it, but a tracked version changes nothing about what the scan found.
  const trackSoftware = (id: string, name: string, version: string) => {
    const sv = latest.current.servers.find((x) => x.id === id)
    if (!sv) return
    const has = sv.software.some((x) => x.name.toLowerCase() === name.toLowerCase())
    const software = has ? sv.software.map((x) => (x.name.toLowerCase() === name.toLowerCase() ? { ...x, version } : x)) : [...sv.software, { name, version, target: '' }]
    void attempt(async () => {
      const saved = await configStore.saveServer({ ...sv, software }, '')
      setConfig((c) => ({ ...c, servers: upsert(c.servers, saved) }))
    })
  }
  const deleteServer = (id: string) =>
    attempt(async () => {
      await configStore.deleteServer(id)
      disconnect(id)
      setConfig((c) => ({ ...c, servers: c.servers.filter((x) => x.id !== id), tasks: c.tasks.map((t) => ({ ...t, serverIds: t.serverIds.filter((x) => x !== id) })) }))
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

  const saveTask = (t: Task) =>
    attempt(async () => {
      const saved = await configStore.saveTask(t)
      setConfig((c) => ({ ...c, tasks: upsert(c.tasks, saved) }))
    })
  const deleteTask = (id: string) =>
    attempt(async () => {
      await configStore.deleteTask(id)
      setConfig((c) => ({ ...c, tasks: c.tasks.filter((x) => x.id !== id) }))
    })

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.ctrlKey) return
      const act = (f: () => void) => {
        e.preventDefault()
        f()
      }
      if (e.key === 'k') act(() => setPalette(true))
      else if (e.key === 't' && sessions[selected]?.state === 'live') act(() => addTab(selected))
      else if (e.key === 'l' && lockRef.current?.enabled) act(() => void lockApi.lock())
      else if (e.key === '1') act(() => setMode('firefight'))
      else if (e.key === '2') act(() => setMode('tasks'))
      else if (e.key === '3') act(() => setMode('admin'))
    }
    // Capture phase: xterm stops key events before they bubble, so shortcuts must be caught first.
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [selected, sessions, addTab])

  const sess = sessions[selected]

  // One ward-board level per server, from the last scan, the last connect, the PHP gap and the open tasks.
  const levels = useMemo(() => {
    const today = new Date(now).toLocaleDateString('en-CA')
    return Object.fromEntries(config.servers.map((s) => [s.id, triage(s, analysis[s.id], config.tasks, sessions[s.id], today)]))
  }, [config.servers, config.tasks, analysis, sessions, now])

  const failed = loadError ? <p className="bad" role="alert" style={{ padding: 24 }}>Could not load your servers: {loadError}</p> : null
  if (lock === null) return <div className="app">{failed}</div>
  if (!loaded) return <div className="app">{lock.locked ? <LockScreen onUnlock={unlock} /> : failed}</div>

  return (
    <>
    <div className="app" inert={lock.locked}>
      <header className="top">
        <h1 className="brand">sshdeck</h1>
        <div className="mode" role="group" aria-label="Job">
          <button aria-pressed={mode === 'firefight'} onClick={() => setMode('firefight')}>
            <Mark kind={mode === 'firefight' ? 'filled' : 'hollow'} />
            <span>A · Firefight</span>
          </button>
          <button aria-pressed={mode === 'tasks'} onClick={() => setMode('tasks')}>
            <Mark kind={mode === 'tasks' ? 'filled' : 'hollow'} />
            <span>B · Tasks</span>
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
        {newer && (
          <button className="btn solid" onClick={() => (setAdminTab('system'), setMode('admin'))} title="A newer version is available">
            Update {newer.latest}
          </button>
        )}
        {lock.enabled && (
          <button className="btn themebtn" onClick={() => void lockApi.lock()} title="Lock now (Ctrl L)">
            Lock
          </button>
        )}
        <button className="btn themebtn" onClick={cycleTheme} title="Switch between system, light and dark" aria-label={`Theme: ${theme}. Switch theme`}>
          Theme: {theme[0].toUpperCase() + theme.slice(1)}
        </button>
      </header>

      {server && (
      <main className="stage" hidden={mode !== 'firefight'}>
        <Rack
          servers={servers}
          vpns={vpns}
          sessions={sessions}
          analysis={analysis}
          levels={levels}
          now={now}
          selected={selected}
          onSelect={setSelected}
          onConnect={(id) => !sessions[id] && requestConnect(id)}
        />
        <div className="center">
          <ConnectBar
            server={server}
            vpn={vpnOf(server, vpns)}
            session={sess}
            onConnect={() => requestConnect(selected)}
            onDisconnect={() => disconnect(selected)}
            onManualDone={() => backend.confirmManualVpn(selected)}
          />
          <TerminalPane
            servers={config.servers}
            sessions={sessions}
            selected={selected}
            backend={backend}
            onTab={(id, tab) => (setActiveDoc((a) => ({ ...a, [id]: null })), patch(id, (s) => ({ ...s, activeTab: tab })))}
            docs={docs}
            activeDoc={activeDoc}
            onDoc={(id, path) => setActiveDoc((a) => ({ ...a, [id]: path }))}
            onCloseDoc={closeDoc}
            onDirty={(id, path, dirty) => setDocs((d) => ({ ...d, [id]: (d[id] ?? []).map((x) => (x.path === path && x.dirty !== dirty ? { ...x, dirty } : x)) }))}
            onAddTab={addTab}
            onCloseTab={closeTab}
          />
        </div>
        <div className="side">
          <div className="sidetabs" role="tablist" aria-label="Side panel">
            <button role="tab" aria-selected={side === 'analysis'} onClick={() => setSide('analysis')}>
              Analysis
            </button>
            <button role="tab" aria-selected={side === 'files'} onClick={() => setSide('files')} title="Browse, transfer and edit files on this server">
              Files
            </button>
          </div>
          {side === 'files' ? (
            <FilesPanel serverId={selected} serverName={server.name} live={live} backend={backend} onEdit={(p) => openDoc(selected, p)} />
          ) : (
            <Analysis server={server} triage={levels[server.id]} data={analysis[selected]} scanning={scanning === selected} error={scanErrors[selected]} connected={live} onExec={(c) => requestExec(selected, c)} onRescan={() => void scan(selected)} onTrack={(n, v) => trackSoftware(selected, n, v)} onCopySsh={() => attempt(() => configStore.copySSH(selected))} connection={<Connection server={server} vpns={vpns} session={sess} />} logins={<Logins server={server} onCopy={(l, f) => attempt(() => configStore.copyLogin(selected, l.id, f))} />} />
          )}
        </div>
      </main>
      )}

      {mode === 'tasks' && (
        <main className="stage">
          <TasksBoard
            tasks={config.tasks}
            servers={servers}
            onSave={saveTask}
            onDelete={deleteTask}
            onOpen={(id) => {
              setSelected(id)
              setMode('firefight')
            }}
          />
        </main>
      )}

      {mode === 'admin' && (
        <main className="stage">
          <Admin servers={servers} vpns={vpns} lock={lock} onLockChange={setLock} onSaveServer={saveServer} onDeleteServer={deleteServer} onSaveVpn={saveVpn} onDeleteVpn={deleteVpn} onSaveLogin={saveLogin} onDeleteLogin={deleteLogin} startTab={adminTab} onImported={() => void configStore.load().then(setConfig).catch((e) => setLoadError(messageOf(e)))} />
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
            if (go && !sessions[id]) requestConnect(id)
          }}
        />
      )}
    </div>
    {ask && !lock.locked && (
      <Confirm
        title={ask.title}
        body={ask.body}
        confirmLabel={ask.label}
        onCancel={() => (ask.cancelled?.(), setAsk(null))}
        onConfirm={() => {
          const run = ask.run
          setAsk(null)
          run()
        }}
      />
    )}
    {lock.locked && <LockScreen onUnlock={unlock} />}
    </>
  )
}
