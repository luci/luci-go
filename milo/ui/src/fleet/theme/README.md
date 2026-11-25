# Fleet Console styling guide

This folder holds the shared design tokens, theme variants, and layout recipes
for the Fleet Console. Components consume shared tokens and recipes from here
rather than inlining one-off hex colors, radii, or shadows; when a new pattern
emerges, add a token or recipe here so the rest of the app can reuse it.

| File | What it holds | When you touch it |
| --- | --- | --- |
| [`palettes.ts`](./palettes.ts) | **The only place with color values.** The `lightPalette` and `darkPalette` objects give every color role a value. | To add or change a color, always in both palettes. |
| [`tokens.ts`](./tokens.ts) | Design tokens: `radii`, `fleetColors`, `statusTones`, `toneColors`, `healthStateColors`, `avatarColors`, `borders`, `elevations`, `fontFamilies`, `fontSizes`, `controlHeights`, `iconSizes`. Color tokens are CSS `var()` references that follow the active theme. | Only to add a missing token, with a comment on when to use it. |
| [`fleet_theme_provider.tsx`](./fleet_theme_provider.tsx), [`fleet_theme_context.ts`](./fleet_theme_context.ts) | `FleetThemeProvider` (picks Light / Dark / Auto, writes the palette as CSS variables, provides the MUI theme) and `useFleetTheme()`. | Rarely. This is the only code that knows which theme is active. |
| [`theme.tsx`](./theme.tsx) | MUI theme: palette, typography, and component variants/overrides (`Button variant="toolbar"`, `Chip variant="filter"`, Tabs, Paper, Card, Tooltip, MenuItem). | When many components need the same look. |
| [`recipes.ts`](./recipes.ts) | `sx` recipes for layouts and interactive tiles the theme can't express: `surfaceCardSx`, `surfaceCardClippedSx`, `tonalPanelSx`, `pageHeaderSx`, `dockedTabStripSx`, `tabPanelSx`, `actionRowSx`, `pillInputSx`, `iconButtonSx`, `metricTileSx`, `subMetricRowSx`, `focusRingSx`. | When a layout or interactive surface pattern repeats on 2+ pages. |
| [`colors.ts`](./colors.ts) | Raw light Material hue scales that back the light `toneColors`. Theme internals only; components can't import it. | Never from a component. |

## Rules

1. **No raw style literals in components.** No hex colors, no
   `borderRadius: '8px'`, no `boxShadow: '0 1px ...'`, no `fontSize: '0.8125rem'`.
   Import a token, a recipe, or use a theme variant. `'50%'` for circles is fine.
   The FCon-only ESLint rule `fcon-rules/no-hardcoded-colors` (in
   `src/fleet/eslint_config.js`) blocks raw color literals and legacy color
   imports at lint time, and `style_conventions.test.ts` fails the build if the
   count of raw literals in `src/fleet` grows.
2. **Three radii only.** `radii.sm` (6px) for small inner elements,
   `radii.md` (12px) for every surface container, `radii.pill` for anything
   pill-shaped (buttons, chips, status pills, search bars).
3. **One surface level.** Content sits in a `surfaceCardSx` card
   (`fleetColors.surface`) on the `fleetColors.canvas` page. Never nest a bordered card directly inside
   another bordered card. Use `tonalPanelSx` for a nested group.
4. **Reach for the shared piece first:**

   | Need | Use |
   | --- | --- |
   | Toolbar or page-header action | `<Button variant="toolbar" startIcon={<Icon />}>` (neutral: add `color="inherit"`) |
   | Applied filter | `<Chip variant="filter">` |
   | Task result, DUT state, health | `<StatusPill tone="success">` or `<TaskResultPill result={...}>` from `@/fleet/components/status_pill/status_pill` |
   | Content card or metrics pane | `surfaceCardSx` or `MetricsContainer` from `@/fleet/constants/css_snippets` |
   | Table | `useFCDataTable` (already styled; don't wrap it in another card) |
   | Page header + tabs | `pageHeaderSx` + `dockedTabStripSx` + `tabPanelSx`; `<TabList>` needs no `sx` |
   | Search input | `pillInputSx` on the `input` slot |
   | Interactive KPI tile or sub-metric row | `metricTileSx` or `subMetricRowSx` (`6px` radius + `surfaceTonal` hover + `2px` focus ring; never use `9999px` pill radius on multi-line blocks) |
   | Text | `<Typography variant="h5 | h6 | body1 | body2 | caption">` |

5. **`sx` at the call site is for layout only:** margin, padding, gap, flex,
   grid, width. If you're setting color, radius, border, shadow, or font size,
   you're in the wrong place.
6. **Accessibility (a11y) & Focus Visibility:** Never suppress keyboard focus
   rings with `outline: 'none'` on `:focus-visible` (use `focusRingSx`,
   `metricTileSx`, or `subMetricRowSx`). Every icon-only `<IconButton>` must
   specify an explicit `aria-label`. Keep all text and status pill foregrounds
   at `>= 4.5:1` WCAG AA contrast without lowering contrast via `opacity`.
7. **Need something new?** Add a token or recipe here (with a usage comment)
   and use it. Don't inline a near-duplicate "just this once."
8. **No `!important`.** Raise specificity with `'&&'` if MUI wins. The only
   exception is Material React Table internals that already use
   `!important` (column header resize handles in
   `fc_data_table/fleet_column_header.styles.ts`).

## Light / Dark / Auto

Fleet Console has a light and a dark theme and defaults to **Auto** (following
the browser/OS color-scheme preference). Users can toggle Light ↔ Dark from the
header icon button or pick **Theme → Light / Dark / Auto** in the ⋮ settings
menu. Explicit choices are stored in localStorage (`fleet-console-theme`), and
the resolved theme is exposed as `data-fleet-theme` on `<html>` for debugging
and screenshots.

How it works:

1. [`palettes.ts`](./palettes.ts) holds every color value, as two objects
   (`lightPalette`, `darkPalette`) with the same semantic roles.
2. [`tokens.ts`](./tokens.ts) turns each role into a CSS variable reference
   with the light value as fallback, e.g.
   `fleetColors.surface === 'var(--fc-surface, #ffffff)'`.
3. `FleetThemeProvider` (in `FleetLayout`) writes the dark palette as
   `--fc-*` custom properties on `:root` when dark is active, along with the
   shared LUCI CSS variables, and provides the matching MUI theme from
   `createFleetTheme(palette)`. Light sets no variables, so it renders exactly
   as before.

Rules:

- **Never branch on the theme in a component.** No `theme.palette.mode`, no
  `isDark`, no `prefers-color-scheme`, no `useFleetTheme()` (only the
  settings menu reads it). If something should look different in dark, give
  it a role whose dark value differs. `style_conventions.test.ts` fails on
  theme branching outside `src/fleet/theme/`.
- **Only theme-aware colors.** Use `fleetColors.*`, `statusTones.*`,
  `toneColors.*`, `healthStateColors.*`, `avatarColors.*`, or MUI palette keys
  (`'text.secondary'`, `'primary.main'`). No hex, `rgb()`, `hsl()`, named
  colors, `@mui/material/colors`, `theme.palette.grey`, or imports of
  `colors.ts` in components; the conventions test enforces this.

Which token?

| Need | Use |
| --- | --- |
| Page, card, header, sidebar backgrounds; borders; text | `fleetColors` (`canvas`, `surface`, `shell`, `border`, `textPrimary`, ...) |
| Task / device state pill | `<StatusPill tone=...>` or `statusTones[tone]` |
| Chips, indicators, chart series with more steps or a decorative hue | `toneColors[tone][role]`, e.g. `toneColors.success.container` + `toneColors.success.onContainer` |
| User avatars (non-inverting saturated fills + white initial) | `avatarColors.fills` + `avatarColors.onFill` |
| ChromeOS health spectrum | `healthStateColors` |

### Add a color

1. Pick a semantic name for what it's for (`infoBanner`, `track`), not what
   it looks like (`lightBlue`).
2. Add it to `FleetColorRoles` in `palettes.ts` with a doc comment, and give it
   a value in **both** `lightPalette.colors` and `darkPalette.colors`. For a
   new tone role, extend `ToneRoles` and both tone builders.
3. Use it as `fleetColors.<name>`. `tokens.test.ts` checks that both palettes
   define every variable and that text and status pairs meet WCAG AA
   (4.5:1) in both themes; add your pair there if it's text.

## Gotchas

- **`<Button variant="toolbar">` ignores `size`.** Use `color="inherit"` for
  the neutral pill; every other `color` renders the primary pill.
- **Material React Table paints row backgrounds with `!important`.** Don't set
  `backgroundColor` on rows. Tint cells (`'& > .MuiTableCell-root'`), or pass
  `mrtTheme` (`baseBackgroundColor`, `selectedRowBackgroundColor`). New tables
  should use `useFCDataTable`, which already does this.
- **Inputs:** text fields get `radii.sm` from the theme. Only search bars use
  `pillInputSx`.
- **`MetricsContainer`** lives in `@/fleet/constants/css_snippets` for
  historical reasons; it is the card recipe for metric panes and uses the same
  tokens as `surfaceCardSx`.
- **Never pass color tokens to `alpha()`, `darken()` or `lighten()`.** They
  are CSS variables, so MUI can't parse them. Use `theme.palette` (concrete
  values for the active theme) or add a role to the palettes.

## Example

```tsx
import { Box, Button, Typography } from '@mui/material';

import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import { actionRowSx, surfaceCardSx } from '@/fleet/theme/recipes';

<Box sx={{ ...surfaceCardSx, p: 3 }}>
  <Typography variant="h6">Lab health</Typography>
  <StatusPill tone="warning">Degraded</StatusPill>
  <Box sx={{ ...actionRowSx, mt: 2 }}>
    <Button variant="toolbar" startIcon={<BuildIcon />}>Run autorepair</Button>
    <Button variant="toolbar" color="inherit">Export</Button>
  </Box>
</Box>;
```
