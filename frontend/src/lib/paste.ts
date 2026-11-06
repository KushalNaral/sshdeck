/** What to type into a terminal for a snippet. Several lines are sent as a bracketed paste, so the shell shows them without running them. */
export const toTerminal = (cmd: string) => (cmd.includes('\n') ? `\x1b[200~${cmd}\x1b[201~` : cmd)

/** Quotes a string for a POSIX shell, so a folder name can never become syntax. */
export const shellQuote = (s: string) => `'${s.replaceAll("'", `'\\''`)}'`
