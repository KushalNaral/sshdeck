import { mockBackend } from './api'
import { wailsBackend } from './wailsBackend'

/** Inside the Wails window everything is real; in a plain browser (npm run dev) the demo backend stands in. */
export const backend = 'go' in window ? wailsBackend : mockBackend
