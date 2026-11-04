import * as App from '../../wailsjs/go/main/App'

export interface AuditEntry {
  ts: string
  actor: string
  event: string
  subject?: string
  detail?: string
}

export interface AuditCheck {
  ok: boolean
  entries: number
  badLine: number
}

export interface AuditApi {
  list(limit: number): Promise<AuditEntry[]>
  verify(): Promise<AuditCheck>
  /** Notes that the user confirmed an action on a production server. */
  confirm(serverId: string, action: 'connect' | 'container'): void
}

const wailsAudit: AuditApi = {
  list: async (n) => ((await App.ListAudit(n)) ?? []) as AuditEntry[],
  verify: async () => (await App.VerifyAudit()) as AuditCheck,
  confirm: (id, action) => void App.RecordConfirm(id, action).catch(() => {}),
}

// Browser-only dev mode: a few made-up entries.
const mockAudit: AuditApi = {
  list: async () => {
    const at = (m: number) => new Date(Date.now() - m * 60000).toISOString()
    return [
      { ts: at(2), actor: 'you', event: 'container.exec', subject: 'Atlas Server', detail: 'container=app user=root sudo=true' },
      { ts: at(5), actor: 'you', event: 'connect', subject: 'Atlas Server', detail: 'direct' },
      { ts: at(9), actor: 'you', event: 'vpn.up', subject: 'HQ FortiClient' },
      { ts: at(12), actor: 'you', event: 'connect.fail', subject: 'Quartz Reports', detail: 'Lab FortiClient did not come up' },
      { ts: at(30), actor: 'you', event: 'unlock', detail: '' },
    ]
  },
  verify: async () => ({ ok: true, entries: 5, badLine: 0 }),
  confirm: () => {},
}

export const auditApi: AuditApi = 'go' in window ? wailsAudit : mockAudit

/** Plain words for the event codes the Go side writes. */
export const EVENT_LABEL: Record<string, string> = {
  'app.start': 'App started',
  'app.stop': 'App closed',
  connect: 'Connected',
  'connect.fail': 'Connect failed',
  'connect.cancel': 'Connect cancelled',
  disconnect: 'Disconnected',
  'vpn.up': 'VPN up',
  'vpn.down': 'VPN down',
  'shell.open': 'Shell opened',
  'container.exec': 'Container shell',
  'server.save': 'Server saved',
  'server.delete': 'Server deleted',
  'vpn.save': 'VPN profile saved',
  'vpn.delete': 'VPN profile deleted',
  'task.save': 'Task saved',
  'task.delete': 'Task deleted',
  'production.confirm': 'Confirmed on production',
  'lock.enable': 'Lock turned on',
  'lock.disable': 'Lock turned off',
  lock: 'Locked',
  unlock: 'Unlocked',
  'unlock.fail': 'Wrong passphrase',
  'passphrase.change': 'Passphrase changed',
  'settings.change': 'Lock settings changed',
}
