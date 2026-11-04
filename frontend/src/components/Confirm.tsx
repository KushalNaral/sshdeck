import { useEffect, useRef } from 'react'

interface Props {
  title: string
  body: string
  confirmLabel: string
  onConfirm: () => void
  onCancel: () => void
}

/** A modal yes/no. Focus starts on Cancel, so pressing Enter never confirms by accident. */
export function Confirm({ title, body, confirmLabel, onConfirm, onCancel }: Props) {
  const cancel = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    cancel.current?.focus()
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onCancel()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onCancel])
  return (
    <div className="lockscreen" style={{ zIndex: 900, background: 'rgb(0 0 0 / 0.55)' }} role="alertdialog" aria-modal="true" aria-label={title}>
      <div className="lockcard">
        <h2 className="name">{title}</h2>
        <p>{body}</p>
        <div className="actions">
          <button ref={cancel} className="btn solid" onClick={onCancel}>
            Cancel
          </button>
          <button className="btn danger" onClick={onConfirm}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
