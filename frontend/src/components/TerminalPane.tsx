import { Plus, X } from 'lucide-react'
import type { Backend } from '../api'
import type { Server, Session } from '../types'
import { Term } from './Term'

interface Props {
  servers: Server[]
  sessions: Record<string, Session>
  selected: string
  backend: Backend
  onTab: (serverId: string, tab: number) => void
  onAddTab: (serverId: string) => void
  onCloseTab: (serverId: string, tab: number) => void
}

export function TerminalPane({ servers, sessions, selected, backend, onTab, onAddTab, onCloseTab }: Props) {
  const live = Object.values(sessions).filter((s) => s.state === 'live')
  const cur = sessions[selected]
  return (
    <>
      <div className="tabs" role="tablist" aria-label="Terminal tabs">
        {cur?.state === 'live' &&
          cur.tabs.map((t) => (
            <button key={t} role="tab" aria-selected={cur.activeTab === t} onClick={() => onTab(selected, t)}>
              <span>Shell {t}</span>
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
        {cur?.state === 'live' && (
          <button aria-label="New shell (Ctrl+T)" title="New shell (Ctrl+T)" onClick={() => onAddTab(selected)}>
            <Plus size={14} strokeWidth={2} />
          </button>
        )}
      </div>
      <div className="plate" role="region" aria-label="Terminal">
        {live.flatMap((s) => {
          const server = servers.find((x) => x.id === s.serverId)!
          return s.tabs.map((t) => <Term key={`${s.serverId}-${t}`} server={server} backend={backend} visible={s.serverId === selected && s.activeTab === t} />)
        })}
        {cur?.state !== 'live' && (
          <div className="empty">
            <strong>{cur?.state === 'connecting' ? 'Opening the path to the shell…' : 'No shell open'}</strong>
            <span>{cur?.state === 'connecting' ? 'The steps above show where it is.' : 'Press Enter on a server, or Connect, and the terminal opens here.'}</span>
          </div>
        )}
      </div>
    </>
  )
}
