import type { Mark as MarkKind } from '../types'
import { STAGES } from '../types'

const LABEL: Record<MarkKind, string> = {
  filled: 'Reachable',
  hollow: 'Not connected',
  struck: 'Problem',
  confirmed: 'Connected',
}

/** Fixed-cell state mark. Shape carries the meaning; colour only reinforces it. */
export function Mark({ kind }: { kind: MarkKind }) {
  const tone = kind === 'struck' ? 'bad' : kind === 'confirmed' ? 'good' : ''
  return (
    <svg className={`cell-mark mark-${kind} ${tone}`} viewBox="0 0 14 14" role="img" aria-label={LABEL[kind]}>
      <title>{LABEL[kind]}</title>
      {kind === 'filled' && <rect x="1" y="1" width="12" height="12" fill="currentColor" />}
      {kind === 'hollow' && <rect x="1.5" y="1.5" width="11" height="11" fill="none" stroke="currentColor" strokeWidth="1.5" />}
      {kind === 'struck' && (
        <>
          <rect x="1.5" y="1.5" width="11" height="11" fill="none" stroke="currentColor" strokeWidth="1.5" />
          <path d="M1 13 13 1" stroke="currentColor" strokeWidth="2" />
        </>
      )}
      {kind === 'confirmed' && (
        <>
          <rect x="1" y="1" width="12" height="12" fill="currentColor" />
          <path d="m3.5 7.2 2.3 2.3 4.7-4.9" fill="none" stroke="var(--plate)" strokeWidth="1.8" />
        </>
      )}
    </svg>
  )
}

/** Five fixed-scale stages of the patch journey. */
export function Journey({ stage }: { stage: number }) {
  return (
    <span className="cells" role="img" aria-label={`Patch journey: ${stage === 0 ? 'not started' : STAGES[stage - 1]}`} title={stage === 0 ? 'Not started' : STAGES[stage - 1]}>
      {STAGES.map((s, i) => (
        <i key={s} className={i < stage ? 'on' : ''} />
      ))}
    </span>
  )
}
