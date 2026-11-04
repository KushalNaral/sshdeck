import { useMemo, useState } from 'react'
import { phpGap } from '../lib/php'
import type { Gap } from '../lib/php'
import { viaLabel } from '../lib/status'
import type { Server, Vpn } from '../types'
import { STAGES } from '../types'
import { Mark } from './Mark'

const WEIGHT: Record<Gap, number> = { branch: 0, patch: 1, current: 2 }
const GAP_TEXT: Record<Gap, string> = { branch: 'Needs upgrade', patch: 'Needs patch', current: 'Current' }

interface Props {
  servers: Server[]
  vpns: Vpn[]
  onStage: (id: string, stage: number) => void
  onOpen: (id: string) => void
}

/** Patch job: every server against its patched PHP target, with the five-stage journey editable in place. */
export function PatchBoard({ servers, vpns, onStage, onOpen }: Props) {
  const [behindOnly, setBehindOnly] = useState(true)
  const rows = useMemo(
    () =>
      servers
        .map((s) => ({ s, gap: phpGap(s.php, s.phpTarget) }))
        .filter((r) => !behindOnly || r.gap !== 'current')
        .sort((a, b) => WEIGHT[a.gap] - WEIGHT[b.gap] || a.s.stage - b.s.stage),
    [servers, behindOnly],
  )
  const behind = servers.filter((s) => phpGap(s.php, s.phpTarget) !== 'current').length
  const upgrades = servers.filter((s) => phpGap(s.php, s.phpTarget) === 'branch').length
  const signed = servers.filter((s) => s.stage === 5).length

  return (
    <section className="pane board" aria-label="Patch campaign">
      <div className="tools">
        <strong>
          {behind} of {servers.length} servers are behind their patched PHP · {upgrades} need a version upgrade · {signed} signed off
        </strong>
        <button className="btn" aria-pressed={behindOnly} onClick={() => setBehindOnly((v) => !v)}>
          {behindOnly ? 'Showing behind target' : 'Showing all servers'}
        </button>
      </div>
      <table>
        <thead>
          <tr>
            <th />
            <th>Server</th>
            <th>VPN</th>
            <th>OS</th>
            <th>PHP now</th>
            <th>Target</th>
            <th>Gap</th>
            <th>Journey (click a cell to set the stage)</th>
            <th>Projects</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {rows.map(({ s, gap }) => (
            <tr key={s.id}>
              <td>
                <Mark kind={gap === 'current' ? 'confirmed' : gap === 'branch' ? 'struck' : 'hollow'} />
              </td>
              <td>
                <strong className="name">{s.name}</strong>
                {s.note && <div className="dim">{s.note}</div>}
              </td>
              <td>{viaLabel(s, vpns)}</td>
              <td>{s.os}</td>
              <td>{s.php}</td>
              <td>{s.phpTarget}</td>
              <td className={gap === 'branch' ? 'bad' : gap === 'current' ? 'good' : ''}>{GAP_TEXT[gap]}</td>
              <td>
                <span className="cells" role="group" aria-label={`Journey for ${s.name}`}>
                  {STAGES.map((label, i) => (
                    <button
                      key={label}
                      className="cellbtn"
                      title={label}
                      aria-label={`${label}${s.stage === i + 1 ? ' (current)' : ''}`}
                      aria-pressed={i < s.stage}
                      onClick={() => onStage(s.id, s.stage === i + 1 ? i : i + 1)}
                    >
                      <i className={i < s.stage ? 'on' : ''} />
                    </button>
                  ))}
                </span>
                <span className="dim" style={{ marginLeft: 8 }}>
                  {s.stage === 0 ? 'Not started' : STAGES[s.stage - 1]}
                </span>
              </td>
              <td>{s.projects.length}</td>
              <td>
                <button className="btn" style={{ color: 'var(--ink)' }} onClick={() => onOpen(s.id)}>
                  Open
                </button>
              </td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td colSpan={10} className="dim">
                Every server is at its patched version.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </section>
  )
}
