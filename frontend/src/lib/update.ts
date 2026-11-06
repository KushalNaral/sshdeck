import * as App from '../../wailsjs/go/main/App'
import { BrowserOpenURL, EventsOn } from '../../wailsjs/runtime/runtime'

export interface UpdateInfo {
  current: string
  latest: string
  available: boolean
  notes: string
  pageUrl: string
  /** False when the release has no build for this system or is not signed: send the user to pageUrl instead. */
  canInstall: boolean
}
export interface UpdateStatus {
  version: string
  autoCheck: boolean
}

export interface UpdateApi {
  status(): Promise<UpdateStatus>
  check(): Promise<UpdateInfo>
  /** Downloads, verifies and installs the newest release. Resolves when it is in place; restart() then starts it. */
  install(onProgress: (line: string) => void): Promise<void>
  restart(): Promise<void>
  setAutoCheck(on: boolean): Promise<void>
  /** The startup check found a newer release. Returns an unsubscribe function. */
  onAvailable(cb: (i: UpdateInfo) => void): () => void
  openPage(url: string): void
}

const wailsUpdate: UpdateApi = {
  status: async () => (await App.UpdateStatus()) as UpdateStatus,
  check: async () => (await App.CheckUpdate()) as unknown as UpdateInfo,
  async install(onProgress) {
    const off = EventsOn('update:progress', (l: string) => onProgress(l))
    try {
      await App.InstallUpdate()
    } finally {
      off()
    }
  },
  restart: () => App.RestartApp(),
  setAutoCheck: (on) => App.SetAutoUpdateCheck(on),
  onAvailable: (cb) => EventsOn('update:available', (i: UpdateInfo) => cb(i)),
  openPage: (url) => BrowserOpenURL(url),
}

// Browser-only dev mode: a pretend release so the screens can be tried.
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
const demo: UpdateInfo = {
  current: 'v0.4.0',
  latest: 'v0.5.0',
  available: true,
  notes: 'Faster scans.\nFixes a reconnect problem on Windows.',
  pageUrl: 'https://github.com/KushalNaral/sshdeck/releases',
  canInstall: true,
}
const mockUpdate: UpdateApi = (() => {
  let auto = true
  return {
    status: async () => ({ version: 'v0.4.0', autoCheck: auto }),
    check: async () => (await sleep(500), demo),
    async install(onProgress) {
      onProgress('Downloading v0.5.0 and checking its signature…')
      await sleep(900)
    },
    restart: async () => {},
    setAutoCheck: async (on) => void (auto = on),
    onAvailable: (cb) => {
      // Only with ?update in the address, so screenshots of the demo data are not cluttered by it.
      const t = location.search.includes('update') ? setTimeout(() => cb(demo), 2500) : undefined
      return () => clearTimeout(t)
    },
    openPage: (url) => void window.open(url, '_blank'),
  }
})()

export const updateApi: UpdateApi = 'go' in window ? wailsUpdate : mockUpdate
