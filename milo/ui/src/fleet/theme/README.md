# Fleet Console styling guide

This folder holds the shared design tokens, theme variants, and layout recipes
for the Fleet Console. Components consume shared tokens and recipes from here
rather than inlining one-off hex colors, radii, or shadows; when a new pattern
emerges, add a token or recipe here so the rest of the app can reuse it.

| File | What it holds | When you touch it |
| --- | --- | --- |
| [`tokens.ts`](./tokens.ts) | Raw design tokens: `radii`, `fleetColors`, `borders`, `elevations`, `fontFamilies`, `fontSizes`, `controlHeights`, `iconSizes`, `statusTones`. | Only to add a missing token, with a comment on when to use it. |
| [`theme.tsx`](./theme.tsx) | MUI theme: palette, typography, and component variants/overrides (`Button variant="toolbar"`, `Chip variant="filter"`, Tabs, Paper, Card, Tooltip, MenuItem). | When many components need the same look. |
| [`recipes.ts`](./recipes.ts) | `sx` recipes for layouts and interactive tiles the theme can't express: `surfaceCardSx`, `surfaceCardClippedSx`, `tonalPanelSx`, `pageHeaderSx`, `dockedTabStripSx`, `tabPanelSx`, `actionRowSx`, `pillInputSx`, `iconButtonSx`, `metricTileSx`, `subMetricRowSx`, `focusRingSx`. | When a layout or interactive surface pattern repeats on 2+ pages. |
| [`colors.ts`](./colors.ts) | Legacy hue scales (`colors.blue[600]` ...). | Don't add to it. Prefer `fleetColors` roles. |

## Rules

1. **No raw style literals in components.** No hex colors, no
   `borderRadius: '8px'`, no `boxShadow: '0 1px ...'`, no `fontSize: '0.8125rem'`.
   Import a token, a recipe, or use a theme variant. `'50%'` for circles is fine.
   `style_conventions.test.ts` fails the build if the count of raw literals in
   `src/fleet` grows.
2. **Three radii only.** `radii.sm` (6px) for small inner elements,
   `radii.md` (12px) for every surface container, `radii.pill` for anything
   pill-shaped (buttons, chips, status pills, search bars).
3. **One surface level.** Content sits in a white `surfaceCardSx` card on the
   `fleetColors.canvas` page. Never nest a bordered card directly inside
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
- **Two color modules:** `fleetColors` (roles, in `tokens.ts`) is what new
  code uses. `colors` (hue scales, in `colors.ts`) is legacy; don't alias it
  as `fleetColors`.

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
