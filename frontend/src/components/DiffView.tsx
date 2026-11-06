import type * as Monaco from 'monaco-editor'
import { useEffect, useRef } from 'react'
import { loadMonaco, wantDark } from '../lib/monaco'

interface Props {
  original: string
  modified: string
  language: string
}

/** Two texts side by side with the changes marked. Read-only. */
export function DiffView({ original, modified, language }: Props) {
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    let ed: Monaco.editor.IStandaloneDiffEditor | undefined
    let models: Monaco.editor.ITextModel[] = []
    let gone = false
    void loadMonaco().then((m) => {
      if (gone || !host.current) return
      ed = m.editor.createDiffEditor(host.current, {
        readOnly: true,
        automaticLayout: true,
        renderSideBySide: true,
        originalEditable: false,
        fontFamily: "'JetBrains Mono Variable', monospace",
        fontSize: 13,
        minimap: { enabled: false },
        scrollBeyondLastLine: false,
        theme: wantDark() ? 'sshdeck-dark' : 'sshdeck-light',
      })
      models = [m.editor.createModel(original, language), m.editor.createModel(modified, language)]
      ed.setModel({ original: models[0], modified: models[1] })
    })
    return () => {
      gone = true
      ed?.dispose()
      models.forEach((x) => x.dispose())
    }
  }, [original, modified, language])
  return <div className="diffhost" ref={host} />
}
