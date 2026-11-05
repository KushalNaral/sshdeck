import type { Server, Vpn } from './types'

// Demo data for the browser build: every name, host and user is made up.
// The real list arrives through the importer; passwords never live in the frontend.
const s = (
  id: string,
  name: string,
  vpn: string | null,
  os: string,
  php: string,
  phpTarget: string,
  webserver: string,
  projects: string[],
  mark: Server['mark'] = 'filled',
  note?: string,
): Server => ({
  id,
  name,
  host: `10.0.${(id.charCodeAt(0) % 40) + 1}.${(id.length * 7) % 200 + 10}`,
  port: 22,
  user: 'deploy',
  auth: 'password',
  vpnId: vpn === 'req' ? null : vpn,
  vpnRequired: vpn === 'req' || undefined,
  os,
  software: [
    { name: 'PHP', version: php, target: phpTarget },
    { name: webserver.split(' ')[0], version: webserver.split(' ')[1], target: '' },
  ],
  projects,
  mark,
  note,
})

export const SERVERS: Server[] = [
  s('atlas', 'Atlas Server', null, 'CentOS 7', '7.4.27', '8.2.35', 'Apache 2.4', ['Community Portal', 'Member Directory', 'Co-op Ledger'], 'filled'),
  s('birch', 'Birch Production', null, 'Ubuntu 22.04.4', '8.2.25', '8.2.35', 'Nginx 1.18', ['Savings Portal', 'Trade Desk', 'Alumni Site', 'Events Site'], 'filled'),
  s('cedar', 'Cedar Server', null, 'Ubuntu 22.04.5', '8.3.19', '8.3.36', 'Nginx 1.18', ['Retail Dashboard'], 'filled'),
  s('delta', 'Delta Production', null, 'Ubuntu 24.04.1', '8.2.35', '8.2.35', 'Nginx 1.24', ['Company Website'], 'filled', 'Also runs Jenkins'),
  s('elm', 'Elm Production', null, 'Ubuntu 22.04', '8.2.35', '8.2.35', 'Apache 2.4', ['Furniture Shop'], 'filled'),
  s('fjord', 'Fjord Server', null, 'Ubuntu 24.04.1', '8.4.27', '8.4.27', 'Nginx 1.24', ['Tender Portal'], 'filled'),
  s('granite', 'Granite Production', null, 'Ubuntu 22.04.5', '8.4.11', '8.2.35', 'Nginx 1.18', ['Booking App'], 'filled'),
  s('web', 'Web Production', null, 'Ubuntu 20.04.6', '8.1.23', '8.2.35', 'Apache 2.4', ['Merchant Site'], 'struck', 'Disk 94% on /var'),
  s('harbor', 'Harbor Server', null, 'Ubuntu 20.04.5', '7.4.3', '8.4.35', 'Apache 2.4', ['Logistics Tracker'], 'filled'),
  s('ivy', 'Ivy Server', null, 'Ubuntu 22.04.5', '8.4.27', '8.4.27', 'Nginx 1.24', ['Capital Fund Site'], 'filled'),
  s('juniper', 'Juniper Server', null, 'Ubuntu 24.04', '8.4.24', '8.4.27', 'Nginx 1.24', ['Health Survey', 'Reports Site'], 'filled'),
  s('pine', 'Pine Monitoring', 'forti-hq', 'Ubuntu 26.04', '8.2.33', '8.2.35', 'Apache 2.4', ['Monitoring'], 'filled'),
  s('quartz', 'Quartz Reports', 'forti-lab', 'Ubuntu 26.04', '8.5.4', '8.5.12', 'Nginx 1.28', ['Reports'], 'struck', '2 failed services'),
  s('ruby', 'Ruby Portal', 'gp-partner', 'CentOS 7', '7.2.34', '8.2.35', 'Apache 2.4', ['Partner Portal'], 'hollow', 'Not patchable: CodeIgniter 2'),
  s('sable', 'Sable Server', 'hill-branch', 'Ubuntu 20.04.6', '8.2.8', '8.2.35', 'Apache 2.4', ['Branch Tools'], 'hollow'),
  s('tundra', 'Tundra Server', 'req', 'Ubuntu 22.04.4', '8.2.27', '8.2.35', 'Nginx 1.18', ['Audit Archive'], 'hollow', 'VPN required, type not recorded'),
]

// Demo VPN profiles: gateways use documentation-range addresses; real ones come from the importer.
export const VPNS: Vpn[] = [
  { id: 'forti-hq', name: 'HQ FortiClient', kind: 'forti', gateway: '203.0.113.10', port: 10443, user: 'monitoring' },
  { id: 'forti-lab', name: 'Lab FortiClient', kind: 'forti', gateway: '203.0.113.20', port: 10443, user: 'infodev' },
  { id: 'gp-partner', name: 'Partner GlobalProtect', kind: 'gp', gateway: 'gp.example.gov', port: 443, user: 'support' },
  { id: 'hill-branch', name: 'Branch Hillstone', kind: 'hillstone', gateway: '203.0.113.30', port: 10443, user: 'sable', note: 'No Linux client: connect it yourself.' },
]

export const serverById = (id: string) => SERVERS.find((x) => x.id === id)
