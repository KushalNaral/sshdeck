export type Theme = 'system' | 'light' | 'dark'
const KEY = 'sshdeck.theme'
export const THEMES: Theme[] = ['system', 'light', 'dark']

export function loadTheme(): Theme {
  try {
    const t = localStorage.getItem(KEY)
    if (t === 'light' || t === 'dark') return t
  } catch {
    /* storage unavailable: follow the system */
  }
  return 'system'
}

/** Light and dark pin the palette; system removes the pin so the OS setting decides. */
export function applyTheme(t: Theme) {
  if (t === 'system') document.documentElement.removeAttribute('data-theme')
  else document.documentElement.setAttribute('data-theme', t)
  try {
    localStorage.setItem(KEY, t)
  } catch {
    /* the choice lasts for this run only */
  }
  window.dispatchEvent(new Event('sshdeck:theme'))
}
