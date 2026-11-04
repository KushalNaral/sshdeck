---
version: 1
slug: "frontend-src-app-tsx"
primary_target: "frontend/src/App.tsx"
related_targets: []
---

---
version: 1
slug: "frontend-src-app-tsx"
primary_target: "frontend/src/App.tsx"
related_targets: []
---

# sshdeck app shell

Scope: the whole desktop app shell (server rack, terminal, analysis, tasks, admin, lock, connect flow). Mode: Operate.

Audience and job: support engineer plus a few teammates, in a bright office by day and a dim room during night incidents; two jobs about equally, Firefight (VPN then SSH then analysis) and Tasks (what still needs doing across the fleet). Light and dark; theme follows the OS unless pinned.

Unresolved: how far the Aero glass goes in the dark theme (Windows 7 had none); whether Segoe UI is substituted by Open Sans where it is not installed.

## Direction contract

THESIS: Windows 7 (Aero), pinned by the user and overriding the dealt Ward Triage Board. A familiar, dense desktop utility: glass title band, Explorer-style navigation pane, command-bar buttons, a black console. Refuses the flat modern SaaS look and anything that reads as a toy. The triage tag stays as the product's own signature on top of it.

OWN-WORLD: Aero blue glass (light-blue gradients with a hard mid-split gloss) for the title band and window frame, white content panes, a pale blue navigation pane. Segoe UI-like text (Open Sans where Segoe is absent) at Windows 7's 12 to 13px density. Buttons are the glossy grey Win7 button turning blue on hover and press; lists use the Explorer selection (pale blue gradient, 1px steel-blue border, lighter hover); scrollbars, inputs and tabs follow Windows 7. The terminal is a classic console: black ground, light grey text. Dark theme is a darker Aero variant.

STORY: The engineer sees at a glance which servers need them first and why, connects, and works in the terminal as in a tool they already know. Tasks read like a file list; closed things stay, stamped CLOSED.

FIRST VIEWPORT: Aero glass band: job switch as command-bar buttons (Firefight, Tasks, Admin), theme, lock. Left navigation pane: VPN gates as Explorer folder headings, servers beneath ordered worst level first, each a selectable list row with a triage tag, name, PROD cell, connection mark and one line "reason, scanned Xm ago"; a strip names the first and last server in view. Centre: Connect bar with its steps, tabs, then the console. Right: details pane (OS, PHP against target, disk and memory bars, failed units, containers) with the reasons behind the level. Primary action: Connect, a Win7 default button.

SIGNATURE: the triage tag (levels 1 to 5, numeral plus fill weight plus colour, glossy badges for 1 to 3 and pale outlined badges for 4 and 5) computed from analysis and tasks, always with a traceable reason. A tag that changes level flashes its ring once; reduced motion removes it.

FORM: Windows 7 Aero, pinned by the user; supersedes Ward Triage Board (seed key 27077afa) for the look, keeping its triage logic and raises (CLOSED stamp, state never by colour alone, guide strip, rank as live state).

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
