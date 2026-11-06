import { useEffect, useRef } from 'react'

interface Props {
  title: string
  body: string
  confirmLabel: string
  /** A third choice between Cancel and the confirm button. */
  altLabel?: string
  onAlt?: () => void
  onConfirm: () => void
  onCancel: () => void
}

/** A modal yes/no. Focus starts on Cancel, so pressing Enter never confirms by accident. */
export function Confirm({ title, body, confirmLabel, altLabel, onAlt, onConfirm, onCancel }: Props) {
  const cancel = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    cancel.current?.focus()
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onCancel()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onCancel])
  return (
    <div className="dimmer" role="alertdialog" aria-modal="true" aria-label={title}>
      <div className="dialog">
        <header>sshdeck</header>
        <div className="body">
          <h2 className="name">{title}</h2>
          <p>{body}</p>
          <div className="actions" style={{ justifyContent: 'flex-end' }}>
            <button ref={cancel} className="btn solid" onClick={onCancel}>
              Cancel
            </button>
            {altLabel && (
              <button className="btn primary" onClick={onAlt}>
                {altLabel}
              </button>
            )}
            <button className="btn danger" onClick={onConfirm}>
              {confirmLabel}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
