import { useEffect, useRef, useState } from 'react'
import { LEVEL_NAME, type Level } from '../lib/triage'

/** The ward-board tag: numeral, fill weight and colour together. A tag that changes level flashes once. */
export function Tag({ level, still }: { level: Level; still?: boolean }) {
  const prev = useRef(level)
  const [flash, setFlash] = useState(false)
  useEffect(() => {
    if (prev.current === level) return
    prev.current = level
    if (still) return
    setFlash(true)
    const t = setTimeout(() => setFlash(false), 500)
    return () => clearTimeout(t)
  }, [level, still])
  return (
    <span className={`tag${flash ? ' flash' : ''}`} data-l={level} role="img" aria-label={`Level ${level}, ${LEVEL_NAME[level]}`} title={`Level ${level}: ${LEVEL_NAME[level]}`}>
      {level}
    </span>
  )
}
