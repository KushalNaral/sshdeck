---
name: sshdeck
description: A Windows 7 Aero desktop utility for a support team's fixed server estate, with a triage tag on top.
colors:
  glass-pale: "#eaf1fb"
  glass-mid: "#b0c8e8"
  frame-blue: "#a9c1e2"
  band-line: "#7f9db9"
  band-ink: "#15233a"
  pane-white: "#ffffff"
  nav-pale: "#e9f0f9"
  ink: "#000000"
  dim-slate: "#4b5563"
  rule-blue-grey: "#c3cfde"
  pane-steel: "#8fa3bf"
  heading-navy: "#1e395b"
  link-blue: "#0066cc"
  glow-azure: "#3399ff"
  button-grey: "#ebebeb"
  button-line: "#707070"
  button-hover-sky: "#bee6fd"
  button-hover-line: "#3c7fb1"
  button-down-blue: "#98d1ef"
  select-pale: "#d3e4f8"
  select-line: "#7da2ce"
  hover-pale: "#ebf3fd"
  hover-line: "#b8d6fb"
  console-black: "#000000"
  console-ink: "#c0c0c0"
  console-dim: "#8a8a8a"
  alarm-red: "#b32017"
  ok-green: "#1a7a3a"
  level-1: "#d13a22"
  level-2: "#ee8a22"
  level-3: "#f0d02a"
  level-4: "#1a7a3a"
  level-5: "#1f5a99"
  dark-frame: "#1b2735"
  dark-band: "#34495f"
  dark-pane: "#1c222a"
  dark-nav: "#202833"
  dark-ink: "#eaf0f7"
  dark-dim: "#a7b3c2"
  dark-link: "#7db7ff"
  dark-alarm: "#ff8f84"
  dark-ok: "#6fd09a"
typography:
  heading:
    fontFamily: "'Segoe UI', 'Selawik', 'Open Sans', system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 600
    lineHeight: "22px"
  title-lock:
    fontFamily: "'Segoe UI', 'Selawik', 'Open Sans', system-ui, sans-serif"
    fontSize: "20px"
    fontWeight: 600
    lineHeight: "28px"
  body:
    fontFamily: "'Segoe UI', 'Selawik', 'Open Sans', system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: "18px"
  label:
    fontFamily: "'Segoe UI', 'Selawik', 'Open Sans', system-ui, sans-serif"
    fontSize: "12px"
    fontWeight: 600
    lineHeight: "16px"
  tag-numeral:
    fontFamily: "'Segoe UI', 'Selawik', 'Open Sans', system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 700
    lineHeight: 1
  console:
    fontFamily: "'JetBrains Mono Variable', monospace"
    fontSize: "14px"
    fontWeight: 400
rounded:
  sm: "2px"
  md: "3px"
  tag: "4px"
  lock-card: "6px"
spacing:
  unit: "8px"
  tight: "4px"
  row: "5px"
  md: "12px"
  lg: "16px"
components:
  button:
    backgroundColor: "{colors.button-grey}"
    textColor: "{colors.ink}"
    rounded: "{rounded.md}"
    height: "26px"
    padding: "0 12px"
  button-hover:
    backgroundColor: "{colors.button-hover-sky}"
  button-down:
    backgroundColor: "{colors.button-down-blue}"
  button-primary:
    backgroundColor: "{colors.button-grey}"
    textColor: "{colors.ink}"
    rounded: "{rounded.md}"
    height: "26px"
    padding: "0 12px"
  list-row:
    backgroundColor: "{colors.pane-white}"
    textColor: "{colors.ink}"
    rounded: "{rounded.md}"
    padding: "5px 8px"
  list-row-selected:
    backgroundColor: "{colors.select-pale}"
  triage-tag:
    rounded: "{rounded.tag}"
    typography: "{typography.tag-numeral}"
    size: "26px"
  input:
    backgroundColor: "{colors.pane-white}"
    textColor: "{colors.ink}"
    height: "26px"
    padding: "0 8px"
  console:
    backgroundColor: "{colors.console-black}"
    textColor: "{colors.console-ink}"
    typography: "{typography.console}"
---

# Design System: sshdeck

## Overview

**Creative North Star: "The Familiar Window"**

sshdeck looks like a Windows 7 Aero application and is meant to be used like one: dense, square-ish, quiet. A glass title band carries the job switch, a pale blue navigation pane lists servers like an Explorer folder tree, white content panes hold details and tasks, and the terminal is a classic black console. The user pinned this world; it replaced an earlier dealt "Ward Triage Board" look and kept that board's triage logic. Type is Open Sans standing in for Segoe UI, at 12 to 13px density.

The one thing the product adds is the triage tag: a numbered badge, levels 1 to 5, that says which server needs the engineer first and always sits beside a written reason. Everything else borrows from the OS the team already knows. The dark theme is the same Aero, darkened, defined by the same token names.

**Key Characteristics:**
- Gloss split gradients (hard step at 46 to 50%) on the band, buttons and tags; nothing else is decorative.
- Dense 12/13px text on an 8px unit, 3px corners, 1px steel-blue frame lines.
- Selection and hover follow Explorer: pale blue gradient plus a 1px border.
- Triage level is carried by numeral, fill weight and colour together, never colour alone.
- Black console with light grey text in JetBrains Mono.

## Colors

A cool blue-grey Aero palette with one azure glow for focus and activity, and a five-step red-to-blue triage ladder as the only saturated colour.

### Primary
- **Aero Glass** (#eaf1fb to #b0c8e8, frame #a9c1e2): the title band and window frame, a gradient with a gloss split at 46 to 50%. Band text is Band Ink (#15233a) with a white glow.
- **Glow Azure** (#3399ff): focus rings, the running connect step, the primary-button halo, and the tag-change flash.

### Secondary
- **Hover Sky** (#bee6fd) and **Press Blue** (#98d1ef) with border #3c7fb1: the button turning blue on hover and press.
- **Explorer Selection** (#d3e4f8, line #7da2ce) and **Explorer Hover** (#ebf3fd, line #b8d6fb): list rows, active mode button, pressed chips, selected tab and palette rows.

### Tertiary (triage ladder)
- **Level 1 Immediate** (#d13a22), **Level 2 Very urgent** (#ee8a22), **Level 3 Urgent** (#f0d02a): glossy solid badges with dark lines.
- **Level 4 Standard** (#1a7a3a) and **Level 5 Clear** (#1f5a99): pale gradient fills with coloured numeral; level 5 has a dashed border.

### Neutral
- **Pane White** (#ffffff), **Nav Pale** (#e9f0f9), **Pane Steel** (#8fa3bf frame line), **Rule Blue-Grey** (#c3cfde row dividers).
- **Ink** (#000000) for text, **Dim Slate** (#4b5563) for secondary text, **Heading Navy** (#1e395b) for headings and labels, **Link Blue** (#0066cc).
- **Console Black** (#000000) with **Console Ink** (#c0c0c0) and **Console Dim** (#8a8a8a).
- Alarm Red (#b32017) and OK Green (#1a7a3a) for text states.
- Dark theme: frame #1b2735, band #34495f, pane #1c222a, nav #202833, ink #eaf0f7, dim #a7b3c2, link #7db7ff, alarm #ff8f84, ok #6fd09a. Same variable names, redefined; the triage levels 1 to 3 are reused unchanged.

### Named Rules
**The Never-Colour-Alone Rule.** Triage level is numeral plus fill weight plus colour; disk and memory are cells plus a number; closed tasks are struck through plus a CLOSED stamp.
**The Glow Is Focus Rule.** Azure (#3399ff) means focus or "running", nothing else.

## Typography

**Display Font:** none; headings use the body face.
**Body Font:** Segoe UI, then Selawik, then Open Sans (400/600/700 via @fontsource), system-ui.
**Label/Mono Font:** JetBrains Mono Variable, terminal and deploy log only.

**Character:** a plain OS-system voice. Hierarchy comes from weight (400/600/700) and navy colour, not size.

### Hierarchy
- **Heading** (600, 15px, 22px): pane headings, server names (15px/600 `.name` in lists is weight only at 13px; the 15px size is headings and the tag numeral).
- **Title (lock)** (600, 20px, 28px): lock screen only.
- **Body** (400, 13px, 18px): everything else; tabular numerals on.
- **Label** (600, 12px, Heading Navy): section headings, column headers, rack gate headings. Secondary lines (reason, scanned time) are 12px Dim Slate.
- **Tag numeral** (700, 15px; 13px in the 20px "why" tags).

### Named Rules
**The Four Sizes Rule.** 12, 13, 15, 20px. A new size needs a reason the scale cannot answer.

## Layout

A three-column stage under a 40px glass band: navigation pane (320 to 380px), centre (connect bar, tabs, console), details pane (320 to 400px), 4px gaps and 4px/6px outer padding. Below 1100px the columns narrow to 280 / flexible / 300px; admin collapses to one column. The tasks board spans all columns. Spacing is an 8px unit with 4px, 5px, 12px and 16px steps; list rows have a 48px minimum height, buttons are 26px, inputs 26px.

Not verified: no 960px-wide capture exists; the narrow behaviour above is from CSS only.

## Elevation & Depth

Depth is gradient and line, not shadow. Surfaces are flat panes with 1px steel-blue frames; gloss (inset 1px white highlight, hard split gradient) provides the Aero feel. Real drop shadows appear only on floating windows: the palette, confirm dialog and lock card.

### Shadow Vocabulary
- **Button gloss** (`box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.65)`): inner highlight on buttons; selected rows use `0.55` white.
- **Default-button halo** (`inset 0 0 0 1px #8fd0f6, 0 0 3px #3399ff`): primary and solid buttons.
- **Glass window** (`0 0 0 4px rgb(201 218 240 / 0.8), 0 10px 30px rgb(0 0 0 / 0.4)`): palette and dialog, with a glass-bordered ring.
- **Lock card** (`inset 0 0 0 1px rgb(255 255 255 / 0.25), 0 10px 30px rgb(0 0 0 / 0.4)`) with 8px backdrop blur.

### Named Rules
**The Flat Panes Rule.** Panes never cast shadows; only overlays do.

## Shapes

Square-ish: 3px corners on buttons, rows, steps and tabs (tabs round the top only), 2px on stamps and scrollbar thumbs, 4px on the triage tag, 6px on the lock card. Panes are square with 1px frames. Disk and memory are ten 8x13px cells. Scrollbars are 17px Windows 7 style with arrow buttons.

## Components

### Buttons
- **Shape:** 3px corners, 26px tall, min 64px wide, 1px grey line (#707070).
- **Default:** grey gloss gradient, black text.
- **Hover / Press:** gradient turns sky blue, then deeper blue with an inset shadow. Focus is a 1px dotted ink ring inset 4px.
- **Primary / solid:** blue border plus halo, weight 600. **Danger:** red gloss, white text. **Disabled:** flat #f4f4f4, grey text.
- Chips are the same button at 24px; pressed state is the selection gradient with an inset shadow and weight 700.

### Rack list (navigation pane)
Gate headings are 12px navy labels with a rule underneath; servers are selectable rows with a tag, a 600-weight name, a PROD stamp, a connection mark and a second line "reason, scanned Xm ago". Servers sort worst level first. A sticky guide strip names the first and last server in view. Row actions show on hover, selection or focus.

### Triage tag (signature)
26px badge (20px in reason lists). Levels 1 to 3 glossy solid gradients, 4 and 5 pale with outlined fill; level 5 dashed. A tag that changes level flashes a 3px azure ring once (480ms, stepped); reduced motion removes it. Always has an aria-label naming level and name.

### Inputs / Fields
26px, 1px #abadb3 line with a darker top edge, white fill; hover border #5794bf; focus swaps to azure border with a 3px glow. Invalid is a red border. Labels sit above in Dim Slate.

### Tabs and mode switch
Tabs sit on a glass command strip; the selected tab is white with a steel border and 600 weight, 4px taller. Mode buttons on the band take the Explorer hover and selection treatments.

### Task rows
Grid of status button, title and meta; hover uses Explorer hover. Done rows stay, with a 4% ink wash, struck title and a CLOSED stamp (12px, 700, tracked, 1px outline).

### Dialogs, palette, lock screen
Opaque window with a glass border and, for dialogs, a glass title strip. The lock screen is an Aero blue radial gradient with one translucent glass card and white text.

### Console
Black ground, #c0c0c0 text, JetBrains Mono 14px, 8px inset, steel-blue frame.

## Do's and Don'ts

### Do:
- **Do** define every colour through the existing tokens in `:root` and give the dark theme the same names.
- **Do** build new interactive states from the Explorer hover and selection pairs and the button gradients.
- **Do** give any new status a text or shape carrier alongside colour.
- **Do** keep text at 12, 13, 15 or 20px.
- **Do** keep the gloss split on new glass or button gradients.

### Don't:
- **Don't** add flat modern SaaS treatments (large radii, soft card shadows, pill buttons); the world refuses them.
- **Don't** put shadows on in-flow panes.
- **Don't** use azure (#3399ff) for decoration; it means focus or running.
- **Don't** colour triage level 4 or 5 as solid glossy badges; the fill weight is the signal.

## Not Canonized

Defects and drift present in the build that are not rules for future surfaces: the dark theme block is duplicated verbatim for the media query and the pinned attribute; several literal hexes bypass tokens (scrollbar greys, deploy log #000/#c0c0c0, danger button gradient, `.board .search` top border, `::selection`); `--focus` is declared but unused (`--glow` does the work); `.lockcard` colours are hard-coded white. Not visually verified: lock screen, Admin screens, command palette, confirm dialog and the dark theme (read from code only); no 960px-wide capture exists. The finish review verdict was ship, scoped to eight fixes.
