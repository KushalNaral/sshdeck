import { useCallback, useEffect, useState } from 'react'
import * as App from '../../wailsjs/go/main/App'
import type { Snippet } from '../types'

export interface SnippetsApi {
  list(): Promise<Snippet[]>
  save(s: Snippet): Promise<Snippet>
  remove(id: string): Promise<void>
}

const wails: SnippetsApi = {
  list: async () => ((await App.ListSnippets()) ?? []) as Snippet[],
  save: async (s) => (await App.SaveSnippet(s as any)) as Snippet,
  remove: (id) => App.DeleteSnippet(id),
}

// Browser-only dev mode: kept in memory.
let demo: Snippet[] = [
  { id: 'd1', name: 'Disk usage', command: 'df -h' },
  { id: 'd2', name: 'Failed services', command: 'systemctl --failed --no-pager' },
  { id: 'd3', name: 'Top memory users', command: 'ps aux --sort=-%mem | head -n 8' },
]
const mock: SnippetsApi = {
  list: async () => [...demo],
  save: async (s) => {
    const n = { ...s, id: s.id || `d${Date.now().toString(36)}` }
    demo = demo.some((x) => x.id === n.id) ? demo.map((x) => (x.id === n.id ? n : x)) : [...demo, n]
    return n
  },
  remove: async (id) => void (demo = demo.filter((x) => x.id !== id)),
}

const impl = 'go' in window ? wails : mock
const changed = () => window.dispatchEvent(new Event('snippets:changed'))

export const snippetsApi: SnippetsApi = {
  list: impl.list,
  save: async (s) => {
    const n = await impl.save(s)
    changed()
    return n
  },
  remove: async (id) => {
    await impl.remove(id)
    changed()
  },
}

/** The saved snippets, and a way to reload them after a change. */
export function useSnippets() {
  const [list, setList] = useState<Snippet[]>([])
  const reload = useCallback(() => void snippetsApi.list().then(setList).catch(() => {}), [])
  useEffect(() => {
    reload()
    window.addEventListener('snippets:changed', reload)
    return () => window.removeEventListener('snippets:changed', reload)
  }, [reload])
  return { list, reload }
}
