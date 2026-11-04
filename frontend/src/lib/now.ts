import { useEffect, useState } from 'react'

/** The current time, refreshed every `ms`, so "scanned 4m ago" stays true on screen. */
export function useNow(ms: number): number {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), ms)
    return () => clearInterval(t)
  }, [ms])
  return now
}
