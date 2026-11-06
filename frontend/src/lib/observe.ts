import { useEffect, useRef, useState } from 'react'
import type { Backend } from '../api'
import type { Alert, Sample } from '../types'
import { pushRing } from './metrics'

/** Alerts as the sampler sees them: firing first, then recently resolved. Kept fresh by events. */
export function useAlerts(backend: Backend): Alert[] {
  const [alerts, setAlerts] = useState<Alert[]>([])
  useEffect(() => {
    let live = true
    void backend.observe.alerts().then((a) => live && setAlerts(a))
    const off = backend.observe.onAlerts(setAlerts)
    return () => {
      live = false
      off()
    }
  }, [backend])
  return alerts
}

export const RING = 180 // 15 minutes of 5 second readings

/**
 * The newest reading of every server plus a short ring of recent ones for the sparklines. A server's ring is
 * filled from the store the first time it shows up, then extended by each event.
 */
export function useSamples(backend: Backend): { latest: Record<string, Sample>; rings: Record<string, Sample[]> } {
  const [latest, setLatest] = useState<Record<string, Sample>>({})
  const [rings, setRings] = useState<Record<string, Sample[]>>({})
  const seen = useRef(new Set<string>())
  useEffect(() => {
    let live = true
    const take = (m: Record<string, Sample>) => {
      if (!live) return
      setLatest(m)
      setRings((r) => {
        const next = { ...r }
        for (const [id, s] of Object.entries(m)) next[id] = pushRing(next[id] ?? [], s, RING)
        return next
      })
      for (const id of Object.keys(m)) {
        if (seen.current.has(id)) continue
        seen.current.add(id)
        const to = Date.now() / 1000
        void backend.observe.range(id, to - 15 * 60, to).then((past) => {
          if (!live) return
          setRings((r) => {
            const have = r[id] ?? []
            const older = past.filter((p) => !have.length || p.t < have[0].t)
            return { ...r, [id]: [...older, ...have].slice(-RING) }
          })
        })
      }
    }
    void backend.observe.latest().then(take)
    const off = backend.observe.onSamples(take)
    return () => {
      live = false
      off()
    }
  }, [backend])
  return { latest, rings }
}
