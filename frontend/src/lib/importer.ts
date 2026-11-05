import * as App from '../../wailsjs/go/main/App'

export interface ImportRow {
  index: number
  line: number
  type: 'vpn' | 'server' | 'login' | ''
  name: string
  action: 'new' | 'update' | 'error'
  hasPassword: boolean
  error?: string
}
export interface ImportPreview {
  token: string
  file: string
  rows: ImportRow[]
}
export interface ImportResult {
  added: number
  updated: number
  failures: string[]
}

/** CSV import and export. Parsing happens in Go: passwords in the file never reach the UI. */
export interface ImportApi {
  /** Opens the file dialog. Null when the user cancels. */
  pick(): Promise<ImportPreview | null>
  apply(token: string, rows: number[]): Promise<ImportResult>
  /** Each returns the saved path, or '' when the user cancels. */
  saveTemplate(): Promise<string>
  exportAll(): Promise<string>
}

const wailsImport: ImportApi = {
  async pick() {
    const p = (await App.PickImport()) as unknown as ImportPreview
    return p?.token ? p : null
  },
  apply: async (token, rows) => {
    const r = (await App.ApplyImport(token, rows)) as unknown as ImportResult
    return { ...r, failures: r.failures ?? [] }
  },
  saveTemplate: () => App.SaveTemplate(),
  exportAll: () => App.ExportCSV(),
}

// Browser-only dev mode: a canned file so the review screen can be seen.
const mockImport: ImportApi = {
  async pick() {
    return {
      token: 'demo',
      file: 'servers.csv',
      rows: [
        { index: 0, line: 2, type: 'vpn', name: 'Office VPN', action: 'new', hasPassword: true },
        { index: 1, line: 3, type: 'server', name: 'Web 1', action: 'new', hasPassword: true },
        { index: 2, line: 4, type: 'server', name: 'Atlas Server', action: 'update', hasPassword: false },
        { index: 3, line: 5, type: 'login', name: 'Main database', action: 'new', hasPassword: true },
        { index: 4, line: 6, type: 'server', name: '', action: 'error', hasPassword: false, error: 'name is required' },
      ],
    }
  },
  apply: async (_t, rows) => ({ added: rows.length, updated: 0, failures: [] }),
  saveTemplate: async () => 'sshdeck-template.csv (demo: nothing was written)',
  exportAll: async () => 'sshdeck-export.csv (demo: nothing was written)',
}

export const importApi: ImportApi = 'go' in window ? wailsImport : mockImport
