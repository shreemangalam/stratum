# Visual Language

This document is the source of truth for Stratum's visual identity.
Consult it before writing any UI component. When in doubt, delete
something.

## Principle

The product is the code. Everything else is chrome. Chrome must recede.

Users open Stratum to see two versions of a file and understand what
changed. Every element that isn't code or diff annotation is either
service to that task or noise. Default to noise until proven otherwise.

## Anti-patterns

The following patterns are forbidden without a written exception in the
design docs:

- Gradients of any kind, including subtle ones on buttons or backgrounds.
- Metric cards in a top row: "1,234 diffs computed | 98% cache hit rate
  | 42ms p95." This distracts from the code and encourages vanity metrics.
- Hero sections with centered headlines over dark backgrounds.
- Bento grids.
- Icons at the start of section titles or card titles.
- Emoji anywhere in the interface, including empty states.
- Rounded corners above 4px on structural containers. Buttons and inputs
  may use 4px. Cards use 0–2px.
- Drop shadows for structure. Use borders.
- Purple, indigo, or teal as a primary brand color.
- Feature callouts using colored background pills.
- "New" or "Beta" badges.
- Any pattern lifted unchanged from shadcn/ui defaults.

If a design instinct produces any of the above, stop and re-derive from
the principle.

## Typography

Typography carries the identity. Two families, no more.

- **Interface:** Inter Variable, with `ui-sans-serif` as fallback. Never
  Poppins, Roboto, or a display face.
- **Monospace:** Commit Mono. Distinctive without being ornamental,
  excellent code readability, free and open. JetBrains Mono is acceptable
  fallback but reads more generic.

Sizes, in rem, tight scale:
0.75, 0.8125, 0.875, 1, 1.125, 1.375, 1.75, 2.25. Do not introduce sizes
between these values.

Weights:
- Interface: 400 body, 500 UI labels, 600 headings. No 300, no 700.
- Monospace: 400 default, 500 for changed tokens. Bold code is not the
  answer.

Line-height: 1.5 body, 1.4 monospace, 1.2 headings.

Letter-spacing: 0 for everything except UI labels below 0.875rem, which
get 0.02em.

## Color

The palette is intentionally narrow. Color is functional, not decorative.

**Neutrals** carry 90% of the interface:
- `surface` — page background. Warm off-white in light, warm near-black
  in dark. Never `#FFFFFF` or `#000000`.
- `surface-raised` — 3% lighter or darker than surface.
- `border` — 12% contrast against surface.
- `foreground` — 90% contrast against surface, primary text.
- `foreground-muted` — 55% contrast, secondary text.
- `foreground-faint` — 35% contrast, tertiary text and inactive states.

**Semantic** colors are reserved for diff states and must not be reused
for chrome:
- `diff-add` — desaturated green.
- `diff-remove` — desaturated red.
- `diff-move` — amber. Rare and deliberate.
- `diff-change` — dim purple-blue. For nodes where value changed but
  structure held.

**No accent color.** Stratum does not have a brand accent that appears on
buttons, links, and highlights. Interactive elements are distinguished by
weight and position, not color. If you find yourself wanting an accent,
the layout is wrong.

Both light and dark themes required. Dark is default; the tool is used by
developers and code reads better on dark. Test both from day one.

## Spacing

4px base unit. Scale: 4, 8, 12, 16, 24, 32, 48, 64. No 6, no 20, no 40.

Density is a feature. When choosing between more padding and more
content, choose content. Users are not staring at empty space, they're
looking for the change on line 47.

## Layout

- Content max width: none for the diff view (full width), 640px for prose
  (design docs, empty states, help).
- Everything snaps to a visible grid. If you can't tell what column an
  element aligns to, it's misaligned.
- Sidebars: fixed pixel widths (240, 320, 400), not percentages.
- No centered layouts on desktop. Content left, actions right, no
  exceptions.

## Components

**Buttons.** Two variants: primary (border + solid fill) and secondary
(border only). No ghost buttons, no icon-only primary actions. Height
32px default, 40px for isolated primary actions.

**Inputs.** 32px height, 1px border, 4px radius, monospace when the input
contains code or paths.

**Cards.** Rare. Group content with a top border and typography, not with
a shadowed rectangle. If a card is used, 1px border, no shadow.

**Tables.** Zebra striping is banned. Row separation is a 1px bottom
border or nothing. Column headers 0.75rem, uppercase, letter-spacing
0.04em, foreground-muted.

**Toasts.** None in v1. If the system needs to tell the user something,
the interface reflects the state.

## The diff view

Primary surface. Rules:

- Two panes, equal width, hairline vertical divider.
- Line numbers in a 4-character monospace gutter, foreground-faint.
- Syntax highlighting from tree-sitter, using a palette that stays inside
  the neutral scale plus semantic hints. Keyword weight, not keyword
  color.
- Diff highlights: `diff-*` colors at 8% opacity background, with a 2px
  left-edge accent bar in full-strength color. Text stays fully readable
  through the highlight.
- Move arrows: 1px SVG bezier curves in `diff-move`, terminating in a 4px
  filled circle at each endpoint. No arrowheads.
- Collapsed regions render as a single row with the collapsed line count,
  monospace, foreground-muted, dashed 1px top and bottom borders.
- Scroll sync anchor is the topmost fully-visible matched node in the
  left pane.

## References

Look at these before building any component:

- Linear (linear.app): typography, density, restraint with color.
- Nova (Panic): a code editor UI that feels like a tool, not an app.
- Difftastic screenshots: reference for diff coloring done right.
- diff.rs: minimal, functional, correct.
- grep.app: dense list UI without feeling cramped.
- Cursor's inline diff view: modern editor's structural change presentation.

Do not look at Vercel product pages, shadcn/ui showcase, or Tailwind
template galleries. Those are the visual sources for the aesthetic this
document exists to prevent.
