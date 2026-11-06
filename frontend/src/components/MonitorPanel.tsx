import { useEffect, useState } from 'react'
import { backend } from '../backend'
import { messageOf } from '../lib/config'
import type { MonitorSettings } from '../types'

const FIELDS: { key: keyof MonitorSettings; label: string; unit: string; min: number; max: number }[] = [
  { key: 'sampleSeconds', label: 'Read connected servers every', unit: 'seconds', min: 2, max: 300 },
  { key: 'scanMinutes', label: 'Scan them again every', unit: 'minutes', min: 1, max: 1440 },
  { key: 'diskWarn', label: 'Disk warning at', unit: '%', min: 50, max: 100 },
  { key: 'diskCrit', label: 'Disk critical at', unit: '%', min: 50, max: 100 },
  { key: 'memWarn', label: 'Memory alert at', unit: '% for 5 minutes', min: 50, max: 100 },
  { key: 'cpuWarn', label: 'CPU alert at', unit: '% for 5 minutes', min: 50, max: 100 },
]

/** Observe settings: how often connected servers are read, and where the alert lines are. */
export function MonitorPanel() {
  const [s, setS] = useState<MonitorSettings | null>(null)
  const [msg, setMsg] = useState<{ text: string; bad: boolean } | null>(null)
  useEffect(() => void backend.observe.settings().then(setS, (e) => setMsg({ text: messageOf(e), bad: true })), [])
  if (!s) return null
  const save = async () => {
    try {
      await backend.observe.saveSettings(s)
      setMsg({ text: 'Saved.', bad: false })
    } catch (e) {
      setMsg({ text: messageOf(e), bad: true })
    }
  }
  return (
    <div className="admin-form" style={{ borderLeft: 0, maxWidth: 760 }}>
      <h2 className="name">Monitoring</h2>
      <p className="dim">
        sshdeck reads the servers you are connected to. It never logs in to a server by itself, so nothing is read from the ones that are not connected.
      </p>
      <label className="check">
        <input type="checkbox" checked={!s.off} onChange={(e) => setS({ ...s, off: !e.target.checked })} />
        <span>Read connected servers and raise alerts</span>
      </label>
      <label className="check">
        <input type="checkbox" checked={!s.noNotify} onChange={(e) => setS({ ...s, noNotify: !e.target.checked })} />
        <span>Show a desktop notification for a new alert</span>
      </label>
      <div className="mon-grid">
        {FIELDS.map((f) => (
          <label key={f.key}>
            <span>{f.label}</span>
            <input type="number" min={f.min} max={f.max} value={s[f.key] as number} onChange={(e) => setS({ ...s, [f.key]: Number(e.target.value) })} />
            <span className="dim">{f.unit}</span>
          </label>
        ))}
      </div>
      <div className="actions">
        <button className="btn solid" onClick={() => void save()}>
          Save
        </button>
        {msg && (
          <span className={msg.bad ? 'bad' : 'good'} role={msg.bad ? 'alert' : 'status'}>
            {msg.text}
          </span>
        )}
      </div>
      <p className="dim">Readings are kept for 7 days in the sshdeck data folder (numbers only, no passwords or host names).</p>
    </div>
  )
}
