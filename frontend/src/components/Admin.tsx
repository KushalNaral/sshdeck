import { useState, type ReactNode } from 'react'
import { viaLabel } from '../lib/status'
import type { Login, Server, Vpn, VpnKind } from '../types'
import { VPN_LABEL } from '../types'
import { AuditPanel } from './AuditPanel'
import { GatewayCert } from './GatewayCert'
import { ImportPanel } from './ImportPanel'
import { LoginsEditor } from './LoginsEditor'
import { ReconnectSetting, SecurityPanel } from './SecurityPanel'
import { SystemPanel } from './SystemPanel'
import { UpdatePanel } from './UpdatePanel'
import type { LockState } from '../lib/lock'
import { Mark } from './Mark'

type Tab = 'servers' | 'vpns' | 'import' | 'system' | 'security' | 'audit'

interface Props {
  servers: Server[]
  vpns: Vpn[]
  lock: LockState
  onLockChange: (s: LockState) => void
  onSaveServer: (s: Server, password: string) => Promise<string | null>
  onDeleteServer: (id: string) => Promise<string | null>
  onSaveVpn: (v: Vpn, password: string) => Promise<string | null>
  onDeleteVpn: (id: string) => Promise<string | null>
  onSaveLogin: (serverId: string, l: Login, secret: string) => Promise<string | null>
  onDeleteLogin: (serverId: string, id: string) => Promise<string | null>
  /** The CSV import changed the stored data: load it again. */
  onImported: () => void
  /** Open on this tab instead of Servers. */
  startTab?: Tab
}

const portOk = (p: number) => Number.isInteger(p) && p >= 1 && p <= 65535

const blankServer = (): Server => ({
  id: '', name: '', host: '', port: 22, user: '', auth: 'password', vpnId: null, os: '', software: [], projects: [], mark: 'hollow',
})
const blankVpn = (): Vpn => ({ id: '', name: '', kind: 'forti', gateway: '', port: 10443, user: '' })

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
    </label>
  )
}

const Note = () => <p className="dim">Passwords go to the OS keyring, never to the data file. This browser demo keeps none.</p>

function ServerForm({ initial, vpns, isNew, onSave, onCancel, onDelete, onDuplicate, logins }: { initial: Server; vpns: Vpn[]; isNew: boolean; onSave: Props['onSaveServer']; onCancel: () => void; onDelete: () => Promise<string | null>; onDuplicate: (s: Server) => void; logins?: ReactNode }) {
  const [d, setD] = useState(initial)
  const [pw, setPw] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [confirm, setConfirm] = useState(false)
  const set = <K extends keyof Server>(k: K, v: Server[K]) => setD((p) => ({ ...p, [k]: v }))

  const save = async () => {
    if (!d.name.trim()) return setErr('Name is required. Give the server a name you will recognise.')
    if (!d.host.trim()) return setErr('Host is required. Enter an IP address or hostname.')
    if (!d.user.trim()) return setErr('User is required. Enter the SSH username.')
    if (!portOk(d.port)) return setErr('Port must be a number from 1 to 65535.')
    const m = await onSave({ ...d, id: d.id, name: d.name.trim(), host: d.host.trim(), user: d.user.trim(), vpnRequired: d.vpnId ? undefined : d.vpnRequired }, pw)
    if (m) setErr(m)
  }

  return (
    <form className="admin-form" onSubmit={(e) => (e.preventDefault(), save())}>
      <h2 className="name">{isNew ? 'New server' : `Edit ${initial.name}`}</h2>
      <Field label="Name"><input value={d.name} onChange={(e) => set('name', e.target.value)} autoFocus /></Field>
      <div className="two">
        <Field label="Host"><input value={d.host} onChange={(e) => set('host', e.target.value)} /></Field>
        <Field label="SSH port"><input type="number" value={d.port} onChange={(e) => set('port', Number(e.target.value))} /></Field>
      </div>
      <div className="two">
        <Field label="User"><input value={d.user} onChange={(e) => set('user', e.target.value)} /></Field>
        <Field label="Sign in with">
          <select value={d.auth} onChange={(e) => set('auth', e.target.value as Server['auth'])}>
            <option value="password">Password</option>
            <option value="key">SSH key</option>
          </select>
        </Field>
      </div>
      {d.auth === 'password' && (
        <Field label="Password">
          <input type="password" autoComplete="new-password" value={pw} placeholder={isNew ? 'Stored in the OS keyring' : 'Leave blank to keep the stored password'} onChange={(e) => setPw(e.target.value)} />
        </Field>
      )}
      {d.auth === 'key' && (
        <>
          <Field label="Key file">
            <input value={d.keyPath ?? ''} placeholder="~/.ssh/id_ed25519, or leave empty to use ssh-agent" onChange={(e) => set('keyPath', e.target.value)} />
          </Field>
          <Field label="Key passphrase">
            <input type="password" autoComplete="new-password" value={pw} placeholder={isNew ? 'Only if the key has one. Stored in the OS keyring' : 'Leave blank to keep the stored passphrase'} onChange={(e) => setPw(e.target.value)} />
          </Field>
        </>
      )}
      <label style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 12 }}>
        <input type="checkbox" checked={!!d.production} onChange={(e) => set('production', e.target.checked)} />
        <span>Production server: ask before connecting and before a root container shell</span>
      </label>
      <label style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 12 }}>
        <input type="checkbox" checked={!!d.autoConnect} onChange={(e) => set('autoConnect', e.target.checked)} />
        <span>Connect when sshdeck starts (one server at a time; production servers still ask)</span>
      </label>
      <Field label="Reached through">
        <select
          value={d.vpnId ?? (d.vpnRequired ? '__req' : '')}
          onChange={(e) => setD((p) => ({ ...p, vpnId: e.target.value && e.target.value !== '__req' ? e.target.value : null, vpnRequired: e.target.value === '__req' || undefined }))}
        >
          <option value="">Direct network</option>
          {vpns.map((v) => (
            <option key={v.id} value={v.id}>{v.name}</option>
          ))}
          <option value="__req">Needs a VPN, not set yet</option>
        </select>
      </Field>
      <Field label="Tracked software">
        <div style={{ display: 'grid', gap: 6 }}>
          {d.software.map((x, i) => {
            const edit = (k: keyof typeof x, v: string) => setD((p) => ({ ...p, software: p.software.map((y, j) => (j === i ? { ...y, [k]: v } : y)) }))
            return (
              <div key={i} style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr 1fr auto', gap: 6 }}>
                <input aria-label="Software name" value={x.name} placeholder="PHP" onChange={(e) => edit('name', e.target.value)} />
                <input aria-label={`${x.name || 'Software'} version now`} value={x.version} placeholder="now 8.2.25" onChange={(e) => edit('version', e.target.value)} />
                <input aria-label={`${x.name || 'Software'} target version`} value={x.target} placeholder="target 8.2.35" onChange={(e) => edit('target', e.target.value)} />
                <button className="btn" type="button" aria-label={`Remove ${x.name || 'software'}`} onClick={() => setD((p) => ({ ...p, software: p.software.filter((_, j) => j !== i) }))}>
                  Remove
                </button>
              </div>
            )
          })}
          <div>
            <button className="btn" type="button" onClick={() => setD((p) => ({ ...p, software: [...p.software, { name: '', version: '', target: '' }] }))}>
              Add software
            </button>
          </div>
        </div>
      </Field>
      {logins}
      <Note />
      {err && <p className="bad" role="alert">{err}</p>}
      <div className="actions">
        <button className="btn solid" type="submit">{isNew ? 'Add server' : 'Save changes'}</button>
        <button className="btn" type="button" onClick={onCancel}>Cancel</button>
        {!isNew && <button className="btn" type="button" onClick={() => onDuplicate(d)} title="Start a new server with these settings. The password and other logins are not copied.">Duplicate</button>}
        {!isNew &&
          (confirm ? (
            <>
              <button className="btn danger" type="button" onClick={async () => { const m = await onDelete(); if (m) { setErr(m); setConfirm(false) } }}>Delete {initial.name}</button>
              <button className="btn" type="button" onClick={() => setConfirm(false)}>Keep it</button>
            </>
          ) : (
            <button className="btn" type="button" style={{ marginLeft: 'auto' }} onClick={() => setConfirm(true)}>Delete…</button>
          ))}
      </div>
    </form>
  )
}

function VpnForm({ initial, isNew, onSave, onCancel, onDelete }: { initial: Vpn; isNew: boolean; onSave: Props['onSaveVpn']; onCancel: () => void; onDelete: () => Promise<string | null> }) {
  const [d, setD] = useState(initial)
  const [pw, setPw] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [confirm, setConfirm] = useState(false)
  const set = <K extends keyof Vpn>(k: K, v: Vpn[K]) => setD((p) => ({ ...p, [k]: v }))

  const save = async () => {
    if (!d.name.trim()) return setErr('Name is required. Name the VPN after where it leads.')
    if (!d.gateway.trim()) return setErr('Gateway is required. Enter the VPN server address.')
    if (!portOk(d.port)) return setErr('Port must be a number from 1 to 65535.')
    const m = await onSave({ ...d, id: d.id, name: d.name.trim(), gateway: d.gateway.trim(), user: d.user.trim() }, pw)
    if (m) setErr(m)
  }

  return (
    <form className="admin-form" onSubmit={(e) => (e.preventDefault(), save())}>
      <h2 className="name">{isNew ? 'New VPN profile' : `Edit ${initial.name}`}</h2>
      <Field label="Name"><input value={d.name} onChange={(e) => set('name', e.target.value)} autoFocus /></Field>
      <Field label="Client">
        <select value={d.kind} onChange={(e) => set('kind', e.target.value as VpnKind)}>
          {(Object.keys(VPN_LABEL) as VpnKind[]).map((k) => (
            <option key={k} value={k}>{VPN_LABEL[k]}</option>
          ))}
        </select>
      </Field>
      <div className="two">
        <Field label="Gateway"><input value={d.gateway} onChange={(e) => set('gateway', e.target.value)} /></Field>
        <Field label="Port"><input type="number" value={d.port} onChange={(e) => set('port', Number(e.target.value))} /></Field>
      </div>
      <Field label="Username"><input value={d.user} onChange={(e) => set('user', e.target.value)} /></Field>
      <Field label="Password">
        <input type="password" autoComplete="new-password" value={pw} placeholder={isNew ? 'Stored in the OS keyring' : 'Leave blank to keep the stored password'} onChange={(e) => setPw(e.target.value)} />
      </Field>
      {d.kind !== 'hillstone' && (
        <>
          <GatewayCert
            gateway={d.gateway}
            port={d.port}
            trustedCert={d.trustedCert}
            trustedPin={d.trustedPin}
            onTrust={(c) => setD((p) => ({ ...p, trustedCert: c.sha256, trustedPin: c.pin }))}
            onClear={() => setD((p) => ({ ...p, trustedCert: '', trustedPin: '' }))}
          />
          <Field label="Trusted certificate (optional)">
            <input value={d.trustedCert ?? ''} placeholder="sha256 digest; filled in by Fetch from gateway, or when a connect is rejected" onChange={(e) => set('trustedCert', e.target.value.trim())} />
          </Field>
        </>
      )}
      {d.kind === 'hillstone' && <p className="dim">Hillstone has no Linux command-line client. sshdeck will ask you to connect it yourself, then confirm.</p>}
      <Note />
      {err && <p className="bad" role="alert">{err}</p>}
      <div className="actions">
        <button className="btn solid" type="submit">{isNew ? 'Add VPN profile' : 'Save changes'}</button>
        <button className="btn" type="button" onClick={onCancel}>Cancel</button>
        {!isNew &&
          (confirm ? (
            <>
              <button className="btn danger" type="button" onClick={async () => { const m = await onDelete(); if (m) { setErr(m); setConfirm(false) } }}>Delete {initial.name}</button>
              <button className="btn" type="button" onClick={() => setConfirm(false)}>Keep it</button>
            </>
          ) : (
            <button className="btn" type="button" style={{ marginLeft: 'auto' }} onClick={() => setConfirm(true)}>Delete…</button>
          ))}
      </div>
    </form>
  )
}

/** Set everything up here: servers, VPN profiles and their credentials. */
export function Admin({ servers, vpns, lock, onLockChange, onSaveServer, onDeleteServer, onSaveVpn, onDeleteVpn, onSaveLogin, onDeleteLogin, onImported, startTab }: Props) {
  const [tab, setTab] = useState<Tab>(startTab ?? 'servers')
  const [editing, setEditing] = useState<string | null>(null) // id, 'new', or null
  const [copyOf, setCopyOf] = useState<Server | null>(null) // a new server starts from this one

  const switchTab = (t: Tab) => (setTab(t), setEditing(null), setCopyOf(null))
  const server = servers.find((s) => s.id === editing)
  const vpn = vpns.find((v) => v.id === editing)

  return (
    <section className="pane board admin" aria-label="Admin">
      <div className="tools">
        <strong>Setup</strong>
        <button className="btn" aria-pressed={tab === 'servers'} onClick={() => switchTab('servers')}>Servers · {servers.length}</button>
        <button className="btn" aria-pressed={tab === 'vpns'} onClick={() => switchTab('vpns')}>VPN profiles · {vpns.length}</button>
        <button className="btn" aria-pressed={tab === 'import'} onClick={() => switchTab('import')}>Import</button>
        <button className="btn" aria-pressed={tab === 'system'} onClick={() => switchTab('system')}>System</button>
        <button className="btn" aria-pressed={tab === 'security'} onClick={() => switchTab('security')}>Security</button>
        <button className="btn" aria-pressed={tab === 'audit'} onClick={() => switchTab('audit')}>Audit</button>
        {(tab === 'servers' || tab === 'vpns') && <button className="btn" style={{ marginLeft: 'auto' }} onClick={() => (setCopyOf(null), setEditing('new'))}>Add {tab === 'servers' ? 'server' : 'VPN profile'}</button>}
      </div>

      {tab === 'import' && <ImportPanel onImported={onImported} />}
      {tab === 'system' && (
        <>
          <UpdatePanel />
          <SystemPanel />
        </>
      )}
      {tab === 'security' && (
        <>
          <ReconnectSetting lock={lock} onChange={onLockChange} />
          <SecurityPanel lock={lock} onChange={onLockChange} />
        </>
      )}
      {tab === 'audit' && <AuditPanel />}
      <div className="admin-body" hidden={tab !== 'servers' && tab !== 'vpns'}>
        <div className="admin-list">
          {tab === 'servers' ? (
            <table>
              <thead><tr><th /><th>Server</th><th>Address</th><th>User</th><th>Reached through</th><th /></tr></thead>
              <tbody>
                {servers.map((s) => (
                  <tr key={s.id} aria-selected={editing === s.id}>
                    <td><Mark kind={s.vpnRequired && !s.vpnId ? 'struck' : 'filled'} /></td>
                    <td><strong className="name">{s.name}</strong></td>
                    <td>{s.host}:{s.port}</td>
                    <td>{s.user} · {s.auth === 'key' ? 'key' : 'password'}</td>
                    <td className={s.vpnRequired && !s.vpnId ? 'bad' : ''}>{viaLabel(s, vpns)}</td>
                    <td><button className="btn" onClick={() => setEditing(s.id)}>Edit</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <table>
              <thead><tr><th /><th>Profile</th><th>Client</th><th>Gateway</th><th>User</th><th>Servers</th><th /></tr></thead>
              <tbody>
                {vpns.map((v) => (
                  <tr key={v.id} aria-selected={editing === v.id}>
                    <td><Mark kind="filled" /></td>
                    <td><strong className="name">{v.name}</strong></td>
                    <td>{VPN_LABEL[v.kind]}</td>
                    <td>{v.gateway}:{v.port}</td>
                    <td>{v.user}</td>
                    <td>{servers.filter((s) => s.vpnId === v.id).length}</td>
                    <td><button className="btn" onClick={() => setEditing(v.id)}>Edit</button></td>
                  </tr>
                ))}
                {vpns.length === 0 && <tr><td colSpan={7} className="dim">No VPN profiles yet. Add one, then point servers at it.</td></tr>}
              </tbody>
            </table>
          )}
        </div>

        {tab === 'servers' && editing && (editing === 'new' || server) && (
          <ServerForm
            key={editing}
            initial={editing === 'new' ? (copyOf ?? blankServer()) : server!}
            onDuplicate={(s) => (setCopyOf({ ...s, id: '', name: `${s.name} copy`, autoConnect: false, logins: undefined }), setEditing('new'))}
            vpns={vpns}
            isNew={editing === 'new'}
            logins={editing !== 'new' && <LoginsEditor logins={server?.logins ?? []} onSave={(l, pw) => onSaveLogin(editing, l, pw)} onDelete={(id) => onDeleteLogin(editing, id)} />}
            onSave={async (s, pw) => { const m = await onSaveServer(s, pw); if (!m) setEditing(null); return m }}
            onCancel={() => setEditing(null)}
            onDelete={async () => {
              const m = await onDeleteServer(editing)
              if (!m) setEditing(null)
              return m
            }}
          />
        )}
        {tab === 'vpns' && editing && (editing === 'new' || vpn) && (
          <VpnForm
            key={editing}
            initial={editing === 'new' ? blankVpn() : vpn!}
            isNew={editing === 'new'}
            onSave={async (v, pw) => { const m = await onSaveVpn(v, pw); if (!m) setEditing(null); return m }}
            onCancel={() => setEditing(null)}
            onDelete={async () => {
              const n = servers.filter((s) => s.vpnId === editing).length
              if (n) return `${n} server${n === 1 ? ' uses' : 's use'} this VPN. Move ${n === 1 ? 'it' : 'them'} to another VPN first.`
              const m = await onDeleteVpn(editing)
              if (!m) setEditing(null)
              return m
            }}
          />
        )}
        {!editing && <div className="admin-form dim">{tab === 'servers' ? 'Pick a server to edit, or add a new one.' : 'Pick a VPN profile to edit, or add a new one.'}</div>}
      </div>
    </section>
  )
}
