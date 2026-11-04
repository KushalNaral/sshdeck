---
version: 1
slug: "frontend-src-app-tsx"
primary_target: "frontend/src/App.tsx"
related_targets: []
---

# sshdeck app shell

Scope: the whole desktop app shell (server list, terminal, analysis, fleet/patch view, connect flow, ODS import review). Mode: Operate.

Audience and job: support engineer plus a few teammates; two jobs about equally, Firefight (VPN then SSH then analysis) and Patch (fleet PHP campaign). Both light and dark; theme follows the OS.

Unresolved: how the hard A/B job switch behaves with keyboard (proposal: Ctrl+1 / Ctrl+2); exact server state marks for Tundra, whose VPN type is unknown.

## Direction contract

THESIS: A 1971 pocket airline timetable on a light table. One type size everywhere; rank is carried by weight, case, reversal and a fixed-cell state mark. Refuses the cards-and-charts admin dashboard and the chrome-heavy SSH client.

OWN-WORLD: Lit goldenrod bed visible between raked slides of slate tinted glass; carbon, ivory, vermilion and bottle green; Bell Gothic-like text with a condensed gothic for labels (system substitutes allowed). Server groups are parallelogram slides raked 11 degrees with counter-sheared contents. State is a mark in a fixed cell: filled holds, hollow does not, vermilion strikes, green confirms. One element per screen is an opaque reversed plate: the terminal. Recognisable with all content removed by the rake, the goldenrod bed and the cell marks.

STORY: The user understands in a glance which servers need them and what state the path to each is in, believes the tool knows their estate, and acts: Connect, then work in the terminal. Patch day reads as a timetable of five-stage journeys.

FIRST VIEWPORT: Top strip: hard A/B switch, Firefight (A) and Patch (B), each a complete screen. Left third: VPN groups as raked slides listing servers, each with a fixed-cell state mark and a one-line sentence. Centre: the terminal as the one opaque reversed plate, widest area, Connect bar above it showing VPN then SSH steps with real durations. Right: analysis in departure-style columns (OS, PHP vs target, disk, memory, failed services, containers). Primary action: Connect, on the selected row and in the Connect bar.

FORM: Timetable Slide Rack (rw-timetable-slide-rack), the lead of the bolder re-roll hand; seed key c56f2eaa, re-roll 2. Raised with the A/B job flip (from the cassette j-card) and the five-stage patch journey (from the botanical folio).

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

Signature interaction: the light travels along the rake bed when state changes. Motion: 90ms two-frame steps hinged at the slide edge; no easing. Reduced motion parks the light.
