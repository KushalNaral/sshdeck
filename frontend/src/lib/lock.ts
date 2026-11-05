import * as App from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

export interface LockState {
  enabled: boolean
  locked: boolean
  idleMinutes: number
  endOnLock: boolean
  /** A dropped connection is redialled by itself. */
  autoReconnect: boolean
}

/** The passphrase lock. Passphrases only ever go one way, into these calls. */
export interface LockApi {
  state(): Promise<LockState>
  /** Resolves with keyring entries that could not be removed. */
  enable(pass: string): Promise<string[]>
  unlock(pass: string): Promise<void>
  lock(): Promise<void>
  /** Goes back to keyring-and-plain-file storage. Needs the passphrase. */
  disable(pass: string): Promise<void>
  change(oldPass: string, newPass: string): Promise<void>
  settings(idleMinutes: number, endOnLock: boolean): Promise<void>
  autoReconnect(on: boolean): Promise<void>
  touch(): void
  /** Go locked the app (idle timeout). Returns an unsubscribe function. */
  onLocked(cb: () => void): () => void
}

const wailsLock: LockApi = {
  state: () => App.LockState() as Promise<LockState>,
  enable: async (p) => (await App.EnableLock(p)) ?? [],
  unlock: (p) => App.Unlock(p),
  lock: () => App.Lock(),
  disable: (p) => App.DisableLock(p),
  change: (o, n) => App.ChangePassphrase(o, n),
  settings: (i, e) => App.SetLockSettings(i, e),
  autoReconnect: (on) => App.SetAutoReconnect(on),
  touch: () => void App.Touch(),
  onLocked: (cb) => EventsOn('app:locked', cb),
}

// Browser-only dev mode: a pretend lock so the screens can be tried without the desktop app.
const mockLock: LockApi = (() => {
  let s: LockState = { enabled: false, locked: false, idleMinutes: 15, endOnLock: false, autoReconnect: true }
  let pass = ''
  const subs = new Set<() => void>()
  return {
    state: async () => ({ ...s }),
    enable: async (p) => {
      pass = p
      s = { ...s, enabled: true }
      return []
    },
    unlock: async (p) => {
      if (p !== pass) throw 'wrong passphrase, or the vault file is damaged'
      s = { ...s, locked: false }
    },
    lock: async () => {
      if (!s.enabled) return
      s = { ...s, locked: true }
      subs.forEach((f) => f())
    },
    disable: async (p) => {
      if (p !== pass) throw 'wrong passphrase, or the vault file is damaged'
      s = { ...s, enabled: false, locked: false }
    },
    change: async (o, n) => {
      if (o !== pass) throw 'wrong passphrase, or the vault file is damaged'
      pass = n
    },
    settings: async (i, e) => void (s = { ...s, idleMinutes: i, endOnLock: e }),
    autoReconnect: async (on) => void (s = { ...s, autoReconnect: on }),
    touch: () => {},
    onLocked: (cb) => (subs.add(cb), () => void subs.delete(cb)),
  }
})()

export const lockApi: LockApi = 'go' in window ? wailsLock : mockLock
