import type * as Monaco from 'monaco-editor'

let loaded: Promise<typeof Monaco> | null = null

/** Monaco is large, so it loads the first time a file is opened, not at start-up. */
export function loadMonaco(): Promise<typeof Monaco> {
  loaded ??= (async () => {
    const [monaco, editorWorker, jsonWorker, cssWorker, htmlWorker, tsWorker] = await Promise.all([
      import('monaco-editor'),
      import('monaco-editor/esm/vs/editor/editor.worker?worker'),
      import('monaco-editor/esm/vs/language/json/json.worker?worker'),
      import('monaco-editor/esm/vs/language/css/css.worker?worker'),
      import('monaco-editor/esm/vs/language/html/html.worker?worker'),
      import('monaco-editor/esm/vs/language/typescript/ts.worker?worker'),
    ])
    self.MonacoEnvironment = {
      getWorker: (_id, label) => {
        if (label === 'json') return new jsonWorker.default()
        if (label === 'css' || label === 'scss' || label === 'less') return new cssWorker.default()
        if (label === 'html' || label === 'handlebars' || label === 'razor') return new htmlWorker.default()
        if (label === 'typescript' || label === 'javascript') return new tsWorker.default()
        return new editorWorker.default()
      },
    }
    defineThemes(monaco)
    return monaco
  })()
  return loaded
}

/** The Windows 7 palette from index.css, as Monaco themes. */
function defineThemes(m: typeof Monaco) {
  m.editor.defineTheme('sshdeck-light', {
    base: 'vs',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': '#ffffff',
      'editor.foreground': '#000000',
      'editor.lineHighlightBackground': '#ebf3fd',
      'editor.selectionBackground': '#d3e4f8',
      'editor.inactiveSelectionBackground': '#e3edf9',
      'editorLineNumber.foreground': '#8fa3bf',
      'editorLineNumber.activeForeground': '#1e395b',
      'editorGutter.background': '#f7faff',
      'editorCursor.foreground': '#0066cc',
      'editorWidget.background': '#e9f0f9',
      'editorWidget.border': '#7f9db9',
      'focusBorder': '#3399ff',
    },
  })
  m.editor.defineTheme('sshdeck-dark', {
    base: 'vs-dark',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': '#1c222a',
      'editor.foreground': '#eaf0f7',
      'editor.lineHighlightBackground': '#283342',
      'editor.selectionBackground': '#2a4260',
      'editor.inactiveSelectionBackground': '#27384f',
      'editorLineNumber.foreground': '#5b6b80',
      'editorLineNumber.activeForeground': '#a9c6ee',
      'editorGutter.background': '#202833',
      'editorCursor.foreground': '#7db7ff',
      'editorWidget.background': '#202833',
      'editorWidget.border': '#435063',
      'focusBorder': '#7db7ff',
    },
  })
}

export function wantDark(): boolean {
  const t = document.documentElement.getAttribute('data-theme')
  return t ? t === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches
}

const byExt: Record<string, string> = { conf: 'ini', cnf: 'ini', env: 'ini', service: 'ini', log: 'plaintext', txt: 'plaintext', tpl: 'html', phtml: 'php', inc: 'php' }

/** Monaco picks a language from the file name; a few server file types it does not know get the nearest one. */
export function languageFor(m: typeof Monaco, path: string): string {
  const base = path.slice(path.lastIndexOf('/') + 1)
  const ext = base.includes('.') ? base.slice(base.lastIndexOf('.') + 1).toLowerCase() : base.replace(/^\./, '').toLowerCase()
  if (byExt[ext]) return byExt[ext]
  const hit = m.languages.getLanguages().find((l) => l.filenames?.includes(base) || l.extensions?.includes('.' + ext))
  return hit?.id ?? 'plaintext'
}
