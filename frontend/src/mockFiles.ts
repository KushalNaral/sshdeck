import type { FilesApi } from './api'
import type { FileEntry, Policy, StartResult, Transfer } from './types'

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
