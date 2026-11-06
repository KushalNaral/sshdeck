import { FileText, Plus, X } from 'lucide-react'
import type { Backend } from '../api'
import type { Server, Session } from '../types'
import { Editor } from './Editor'
import { Term } from './Term'

/** What tells one open file from another: where it is and its path. */
export const docKey = (sid: string, d: Doc) => `${d.target || sid}|${d.path}`

export interface Doc {
  path: string
  /** Where the file is: the server id, or "serverId@container". Empty means the server itself. */
  target?: string
  dirty: boolean
  /** Counts up when something asks to open this file's history. */
  history?: number
}

interface Props {
  servers: Server[]
  sessions: Record<string, Session>
  selected: string
  backend: Backend
  onTab: (serverId: string, tab: number) => void
  onAddTab: (serverId: string) => void
  onCloseTab: (serverId: string, tab: number) => void
  /** Files open in the editor, per server, and the one in front (null when a shell is). */
  docs: Record<string, Doc[]>
  activeDoc: Record<string, string | null>
  onDoc: (serverId: string, key: string) => void
  onCloseDoc: (serverId: string, key: string) => void
  onDirty: (serverId: string, key: string, dirty: boolean) => void
  onHistoryCopy: (serverId: string, mode: '' | 'on' | 'off') => void
}

export function TerminalPane({ servers, sessions, selected, backend, onTab, onAddTab, onCloseTab, docs, activeDoc, onDoc, onCloseDoc, onDirty, onHistoryCopy }: Props) {
  const live = Object.values(sessions).filter((s) => s.state === 'live')
  const cur = sessions[selected]
  const myDocs = docs[selected] ?? []
  const front = activeDoc[selected] ?? null
  return (
    <>
      <div className="tabs" role="tablist" aria-label="Terminal tabs">
        {cur?.state === 'live' &&
          cur.tabs.map((t) => (
            <button key={t} role="tab" aria-selected={!front && cur.activeTab === t} onClick={() => onTab(selected, t)}>
              <span>{cur.tabInfo?.[t] ? `${cur.tabInfo[t].container} · ${cur.tabInfo[t].user || 'default user'}` : `Shell ${t}`}</span>
              {cur.tabs.length > 1 && (
                <span
                  role="button"
                  aria-label={`Close shell ${t}`}
                  onClick={(e) => {
                    e.stopPropagation()
                    onCloseTab(selected, t)
                  }}
                >
                  <X size={14} strokeWidth={2} />
                </span>
              )}
            </button>
          ))}
        {myDocs.map((d) => (
          <button key={docKey(selected, d)} role="tab" aria-selected={front === docKey(selected, d)} onClick={() => onDoc(selected, docKey(selected, d))} title={`${d.target?.includes('@') ? d.target.split('@')[1] + ': ' : ''}${d.path}`}>
            <FileText size={14} strokeWidth={2} />
            <span>
              {d.target?.includes('@') && <span className="dim">{d.target.split('@')[1]}: </span>}
              {d.path.slice(d.path.lastIndexOf('/') + 1)}
              {d.dirty && <span className="dot" role="img" aria-label="unsaved changes" />}
            </span>
            <span
              role="button"
              aria-label={`Close ${d.path.slice(d.path.lastIndexOf('/') + 1)}`}
              onClick={(e) => {
                e.stopPropagation()
                onCloseDoc(selected, docKey(selected, d))
              }}
            >
              <X size={14} strokeWidth={2} />
            </span>
          </button>
        ))}
        {cur?.state === 'live' && (
          <button aria-label="New shell (Ctrl+T)" title="New shell (Ctrl+T)" onClick={() => onAddTab(selected)}>
            <Plus size={14} strokeWidth={2} />
          </button>
        )}
      </div>
      <div className="plate" role="region" aria-label="Terminal">
        {live.flatMap((s) => {
          const server = servers.find((x) => x.id === s.serverId)!
          return s.tabs.map((t) => <Term key={`${s.serverId}-${t}`} server={server} tab={t} container={s.tabInfo?.[t]} backend={backend} visible={s.serverId === selected && s.activeTab === t && !activeDoc[s.serverId]} />)
        })}
        {Object.entries(docs).flatMap(([sid, list]) => list.map((d) => <Editor key={`${sid}:${docKey(sid, d)}`} target={d.target || sid} server={servers.find((x) => x.id === sid)!} live={sessions[sid]?.state === 'live'} historyAsk={d.history ?? 0} onCopy={(m) => onHistoryCopy(sid, m)} path={d.path} backend={backend} visible={sid === selected && activeDoc[sid] === docKey(sid, d)} onDirty={(x) => onDirty(sid, docKey(sid, d), x)} />))}
        {cur?.state !== 'live' && !front && (
          <div className="empty">
            <strong>{cur?.state === 'connecting' ? 'Opening the path to the shell…' : 'No shell open'}</strong>
            <span>{cur?.state === 'connecting' ? 'The steps above show where it is.' : 'Press Enter on a server, or Connect, and the terminal opens here.'}</span>
          </div>
        )}
      </div>
    </>
  )
}
