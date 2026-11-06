import type { FilesApi } from './api'
import type { HistoryApi } from './api'
import type { FileEntry, HistorySettings, Policy, StartResult, Transfer, Version } from './types'

/** Demo file tree for the browser preview: one small site, the same on every server. */
const T0 = 1_760_000_000
const files = new Map<string, string | null>() // null = folder
const seed = (p: string, body: string | null) => files.set(p, body)
seed('/home/deploy', null)
seed('/home/deploy/site', null)
seed('/home/deploy/site/public', null)
seed('/home/deploy/site/public/index.php', "<?php\n\ndeclare(strict_types=1);\n\nrequire __DIR__ . '/../vendor/autoload.php';\n\n$app = require __DIR__ . '/../bootstrap/app.php';\n$app->run();\n")
seed('/home/deploy/site/public/style.css', 'body {\n  margin: 0;\n  font: 14px/1.5 system-ui;\n}\n')
seed('/home/deploy/site/composer.json', '{\n  "name": "demo/site",\n  "require": { "php": "^8.2" }\n}\n')
seed('/home/deploy/site/.env', 'APP_ENV=production\nAPP_DEBUG=false\n')
seed('/home/deploy/logs', null)
seed('/home/deploy/logs/error.log', '[06-Nov 09:14:02] PHP Warning: Undefined variable $user in index.php on line 12\n')
seed('/home/deploy/notes.md', '# Notes\n\n- renew the certificate\n- rotate logs\n')
seed('/home/deploy/backup.tar.gz', '\u0000binary')
seed('/etc', null)
seed('/etc/nginx', null)
seed('/etc/nginx/nginx.conf', 'worker_processes auto;\n\nevents {\n  worker_connections 1024;\n}\n')

const dirOf = (p: string) => p.slice(0, p.lastIndexOf('/')) || '/'
const nameOf = (p: string) => p.slice(p.lastIndexOf('/') + 1)
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
let ver = 1
const versions = new Map<string, string>()
const versionOf = (p: string) => versions.get(p) ?? (versions.set(p, String(ver++)), versions.get(p)!)

function entry(p: string): FileEntry {
  const body = files.get(p)
  return { name: nameOf(p), path: p, dir: body === null, link: false, size: body ? body.length : 0, mode: body === null ? 'rwxr-xr-x' : 'rw-r--r--', mtime: T0 + (p.length % 40) * 86400 }
}

let queue: Transfer[] = []
const subs = new Set<(t: Transfer[]) => void>()
const emit = () => subs.forEach((f) => f(queue))
let seq = 0

function run(name: string, up: boolean, total: number, fail = false) {
  const t: Transfer = { id: `t${++seq}`, serverId: '', name, up, done: 0, total, state: 'running' }
  queue = [...queue, t]
  emit()
  const iv = setInterval(() => {
    const cur = queue.find((x) => x.id === t.id)
    if (!cur || cur.state !== 'running') return clearInterval(iv)
    cur.done = Math.min(cur.total, cur.done + cur.total / 12)
    if (fail && cur.done > cur.total / 2) Object.assign(cur, { state: 'failed', error: 'connection lost' })
    else if (cur.done >= cur.total) cur.state = 'done'
    queue = queue.map((x) => ({ ...x }))
    emit()
    if (cur.state !== 'running') clearInterval(iv)
  }, 250)
}

const started = (n: number): StartResult => ({ started: n })
export const mockFiles: FilesApi = {
  async containers() {
    return ['app', 'queue-worker', 'redis']
  },
  async search(_s, dir, query, byContent) {
    await new Promise((r) => setTimeout(r, 150))
    return byContent
      ? [
          { path: `${dir}/conf/nginx.conf`, line: 12, text: `server_name ${query}.example.org;` },
          { path: `${dir}/app/.env`, line: 3, text: `APP_HOST=${query}` },
        ]
      : [{ path: `${dir}/conf/${query}.conf` }, { path: `${dir}/logs/${query}.log` }]
  },
  async list(_s, dir) {
    await sleep(120)
    const d = dir || '/home/deploy'
    if (files.get(d) !== null) throw new Error(`${d} does not exist`)
    const out = [...files.keys()].filter((p) => p !== d && dirOf(p) === d).map(entry)
    out.sort((a, b) => Number(b.dir) - Number(a.dir) || a.name.toLowerCase().localeCompare(b.name.toLowerCase()))
    return { path: d, entries: out }
  },
  async read(_s, p) {
    await sleep(120)
    const body = files.get(p)
    if (body == null) throw new Error('that is a folder')
    if (body.includes('\u0000')) throw new Error('this does not look like a text file. Download it instead')
    return { path: p, content: body, version: versionOf(p), mode: 'rw-r--r--' }
  },
  async write(_s, p, content, version) {
    await sleep(200)
    if (version && version !== versionOf(p)) throw new Error('changed on the server since you opened it')
    note(p, content, 'save')
    files.set(p, content)
    versions.set(p, String(ver++))
    return versionOf(p)
  },
  async mkdir(_s, p) {
    files.set(p, null)
  },
  async newFile(_s, p) {
    if (files.has(p)) throw new Error(`${nameOf(p)} already exists`)
    files.set(p, '')
  },
  async rename(_s, from, to) {
    if (files.has(to)) throw new Error(`${nameOf(to)} already exists`)
    for (const k of [...files.keys()]) if (k === from || k.startsWith(from + '/')) (files.set(to + k.slice(from.length), files.get(k)!), files.delete(k))
  },
  async remove(_s, paths) {
    for (const p of paths) for (const k of [...files.keys()]) if (k === p || k.startsWith(p + '/')) files.delete(k)
  },
  async pickUpload(_s, _d, folder) {
    run(folder ? 'assets' : 'report-q3.pdf', true, 4_800_000)
    return started(1)
  },
  async upload(_s, _d, locals, policy: Policy) {
    if (policy === 'ask' && locals.some((l) => l.endsWith('index.php'))) return { started: 0, conflicts: ['index.php'], paths: locals }
    locals.forEach((l) => run(nameOf(l), true, 1_200_000))
    return started(locals.length)
  },
  async pickDownload(_s, remotes) {
    remotes.forEach((r) => run(nameOf(r), false, 900_000, nameOf(r) === 'error.log'))
    return started(remotes.length)
  },
  async download(_s, remotes) {
    remotes.forEach((r) => run(nameOf(r), false, 900_000))
    return started(remotes.length)
  },
  cancel(id) {
    queue = queue.map((x) => (x.id === id && x.state === 'running' ? { ...x, state: 'cancelled' } : x))
    emit()
  },
  retry(id) {
    const t = queue.find((x) => x.id === id)
    if (t) run(t.name, t.up, t.total)
  },
  clear() {
    queue = queue.filter((x) => x.state === 'running')
    emit()
  },
  onTransfers(cb) {
    subs.add(cb)
    cb(queue)
    return () => void subs.delete(cb)
  },
  onDrop() {
    return () => {}
  },
}

// ---- demo history ----
const NOW = Date.now()
const vers: Version[] = []
const texts = new Map<string, string>()
let vseq = 0
function add(path: string, body: string, kind: Version['kind'], author: string, login: string, agoMin: number, extra: Partial<Version> = {}) {
  const id = `v${++vseq}`
  texts.set(id, body)
  vers.push({ id, server: '', path, time: NOW - agoMin * 60_000, author, login, kind, hash: id, size: body.length, ...extra })
}
const ME = 'rome@laptop'
add('/home/deploy/site/public/index.php', "<?php\nrequire 'vendor/autoload.php';\n(new App)->run();\n", 'initial', '', 'deploy', 60 * 24 * 5)
add('/home/deploy/site/public/index.php', "<?php\ndeclare(strict_types=1);\nrequire 'vendor/autoload.php';\n(new App)->run();\n", 'save', 'sam@desk', 'deploy', 60 * 30, { remote: true })
add('/home/deploy/site/public/index.php', files.get('/home/deploy/site/public/index.php') ?? '', 'outside', '', 'deploy', 60 * 3)
add('/etc/nginx/nginx.conf', 'worker_processes 1;\n', 'initial', '', 'deploy', 60 * 48)
add('/etc/nginx/nginx.conf', files.get('/etc/nginx/nginx.conf') ?? '', 'save', ME, 'deploy', 25, { pinned: true, note: 'before the TLS change' })

function note(path: string, body: string, kind: Version['kind']) {
  if (vers.filter((v) => v.path === path).length === 0) add(path, files.get(path) ?? '', 'initial', '', 'deploy', 1)
  if (texts.get(vers.filter((v) => v.path === path).sort((a, b) => b.time - a.time)[0]?.id ?? '') === body) return
  add(path, body, kind, ME, 'deploy', 0)
}

let settings: HistorySettings = { off: false, noServerCopy: false, keep: 50, days: 90, deny: [] }
const forServer = (s: string) => vers.map((v) => ({ ...v, server: s }))
export const mockHistory: HistoryApi = {
  async file(s, p) {
    await sleep(80)
    return forServer(s).filter((v) => v.path === p).sort((a, b) => b.time - a.time)
  },
  async server(s, n) {
    await sleep(80)
    return forServer(s).sort((a, b) => b.time - a.time).slice(0, n)
  },
  async content(_s, id) {
    return texts.get(id) ?? ''
  },
  async pin(_s, id, pinned, noteText) {
    const v = vers.find((x) => x.id === id)
    if (v) Object.assign(v, { pinned, note: noteText })
  },
  async restore(_s, p, id) {
    const body = texts.get(id) ?? ''
    note(p, body, 'restore')
    files.set(p, body)
    versions.set(p, String(ver++))
    return versionOf(p)
  },
  async remove(_s, p) {
    for (let i = vers.length - 1; i >= 0; i--) if (!p || vers[i].path === p) vers.splice(i, 1)
  },
  async settings() {
    return settings
  },
  async saveSettings(s) {
    settings = s
  },
  async setCopy() {},
}
