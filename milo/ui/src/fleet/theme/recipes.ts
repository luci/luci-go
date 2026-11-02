// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * Shared `sx` recipes for Fleet Console layouts that MUI theme variants cannot
 * express on their own (page headers, surface cards, tab strips, search bars).
 *
 * Use them as-is or spread them and add layout-only props (margin, gap, flex):
 *
 *   <Box sx={surfaceCardSx}>...</Box>
 *   <Box sx={{ ...surfaceCardSx, p: 3 }}>...</Box>
 *
 * Do not override their colors, radii, borders, or shadows at the call site.
 * If a page needs a different look, add a new recipe here.
 */

import { SxProps, Theme } from '@mui/material/styles';

import {
  borders,
  controlHeights,
  elevations,
  fleetColors,
  fontFamilies,
  fontSizes,
  radii,
} from './tokens';

/** White rounded card on the canvas. Use for every content container. */
export const surfaceCardSx = {
  backgroundColor: fleetColors.surface,
  border: borders.default,
  borderRadius: radii.md,
  boxShadow: elevations.card,
} as const satisfies SxProps<Theme>;

/**
 * Same as `surfaceCardSx` but clips children to the rounded corners. Use when
 * the card holds a table or a full-bleed header.
 */
export const surfaceCardClippedSx = {
  ...surfaceCardSx,
  overflow: 'hidden',
} as const satisfies SxProps<Theme>;

/** Tonal nested panel inside a card (e.g. a summary strip). No shadow. */
export const tonalPanelSx = {
  backgroundColor: fleetColors.surfaceTonal,
  border: borders.divider,
  borderRadius: radii.md,
} as const satisfies SxProps<Theme>;

/**
 * Full-bleed white page header (title row + actions) that sits above a docked
 * tab strip. Pair with `dockedTabStripSx`.
 */
export const pageHeaderSx = {
  backgroundColor: fleetColors.surface,
  px: 3,
  pt: 2.5,
  pb: 1.5,
  display: 'flex',
  flexDirection: 'column',
  gap: 1.5,
} as const satisfies SxProps<Theme>;

/** Container for `<TabList>` / `<Tabs>` docked under a page header. */
export const dockedTabStripSx = {
  backgroundColor: fleetColors.surface,
  borderBottom: borders.default,
  px: 3,
} as const satisfies SxProps<Theme>;

/** Padding for the content area under a docked tab strip. */
export const tabPanelSx = {
  px: 3,
  py: 2.5,
} as const satisfies SxProps<Theme>;

/** Row of 32px action buttons (`<Button variant="toolbar">`). */
export const actionRowSx = {
  display: 'flex',
  flexDirection: 'row',
  alignItems: 'center',
  flexWrap: 'wrap',
  gap: 1,
} as const satisfies SxProps<Theme>;

/**
 * Pill-shaped outlined input. Apply to the `input` slot of a `TextField`
 * (`slotProps={{ input: { sx: pillInputSx } }}`).
 *
 * Uses half of `controlHeights.input` (`20px`) rather than `radii.pill`
 * (`9999px`) so single-line `40px` inputs render as an exact pill while
 * multi-line wrapped filter bars keep a fixed `20px` corner radius instead of
 * expanding their semicircular ends to `50%` of the multi-line height.
 */
export const pillInputSx = {
  minHeight: controlHeights.input,
  backgroundColor: fleetColors.surface,
  borderRadius: `${controlHeights.input / 2}px`,
  fontSize: fontSizes.label,
  boxShadow: elevations.input,
  '&.MuiOutlinedInput-root fieldset': {
    borderColor: fleetColors.border,
    borderRadius: `${controlHeights.input / 2}px`,
  },
  '&.MuiOutlinedInput-root:hover fieldset': {
    borderColor: fleetColors.borderHover,
  },
  '&.MuiOutlinedInput-root.Mui-focused fieldset': {
    borderColor: fleetColors.primary,
    borderWidth: '1.5px',
  },
} as const satisfies SxProps<Theme>;

/** Round 32px icon button with the shared tonal hover and focus ring. */
export const iconButtonSx = {
  width: controlHeights.button,
  height: controlHeights.button,
  color: fleetColors.textSecondary,
  borderRadius: radii.pill,
  '&:hover': {
    backgroundColor: fleetColors.primaryContainer,
  },
  '&:focus-visible, &.Mui-focusVisible': {
    outline: `2px solid ${fleetColors.primary}`,
    outlineOffset: 2,
  },
} as const satisfies SxProps<Theme>;

/** Accessible WCAG 2.4.7 keyboard focus ring. Never set `outline: 'none'`. */
export const focusRingSx = {
  '&:focus-visible, &.Mui-focusVisible': {
    outline: `2px solid ${fleetColors.primary}`,
    outlineOffset: 2,
  },
} as const satisfies SxProps<Theme>;

/**
 * Interactive scorecard KPI tile (e.g. `SingleMetric` or `PlatformSummaryCard` count).
 * Uses a 6px (`radii.sm`) rounded rectangle (never a 9999px pill on multi-line blocks),
 * shared tonal hover fill, and an accessible 2px primary focus ring.
 */
export const metricTileSx = {
  borderRadius: radii.sm,
  transition: 'background-color 0.15s ease',
  '&:hover': {
    backgroundColor: fleetColors.surfaceTonal,
  },
  '&:focus-visible, &.Mui-focusVisible': {
    backgroundColor: fleetColors.surfaceTonal,
    outline: `2px solid ${fleetColors.primary}`,
    outlineOffset: 2,
  },
} as const satisfies SxProps<Theme>;

/**
 * Compact interactive sub-metric or breakdown row (e.g. `SmallMetricItem`, health slice badges).
 * Uses a 6px (`radii.sm`) rounded rectangle, shared tonal hover fill, and a 2px focus ring.
 */
export const subMetricRowSx = {
  borderRadius: radii.sm,
  transition: 'background-color 0.15s ease',
  '&:hover': {
    backgroundColor: fleetColors.surfaceTonal,
  },
  '&:focus-visible, &.Mui-focusVisible': {
    backgroundColor: fleetColors.surfaceTonal,
    outline: `2px solid ${fleetColors.primary}`,
    outlineOffset: 2,
  },
} as const satisfies SxProps<Theme>;

/** Outlined text field inside a date/time filter dropdown. */
export const datePickerTextFieldSx = {
  width: '100%',
  '& .MuiOutlinedInput-root, & .MuiPickersOutlinedInput-root': {
    borderRadius: radii.sm,
    backgroundColor: fleetColors.surface,
    '& fieldset': {
      borderColor: fleetColors.border,
    },
    '&:hover fieldset': {
      borderColor: fleetColors.borderEmphasis,
    },
    '&.Mui-focused fieldset': {
      borderColor: fleetColors.primary,
    },
  },
  '& .MuiInputLabel-root': {
    color: fleetColors.textSecondary,
    fontWeight: 500,
    '&.Mui-focused': {
      color: fleetColors.primary,
    },
  },
} as const satisfies SxProps<Theme>;

/**
 * Selectable pill chip for quick relative presets inside `DateFilter`.
 * Uses `&&:hover` to override `theme.tsx`'s `&.MuiChip-clickable:hover` rule on
 * outlined chips so hovering a selected preset keeps its primary tonal tint.
 */
export const datePresetChipSx = {
  flex: '0 1 auto',
  height: 28,
  borderRadius: radii.pill,
  fontSize: fontSizes.caption,
  padding: 0,
  borderColor: fleetColors.border,
  backgroundColor: 'transparent',
  color: fleetColors.textPrimary,
  fontWeight: 400,
  transition: 'all 0.15s ease',
  '& .MuiChip-label, & .MuiChip-labelSmall': {
    paddingLeft: '5px',
    paddingRight: '5px',
  },
  '&&:hover': {
    backgroundColor: fleetColors.primaryContainer,
    borderColor: fleetColors.borderHover,
  },
  '&[aria-pressed="true"]': {
    borderColor: fleetColors.primary,
    backgroundColor: fleetColors.primaryContainer,
    color: fleetColors.primary,
    fontWeight: 500,
    '&&:hover': {
      backgroundColor: fleetColors.primaryContainerHover,
      borderColor: fleetColors.primary,
    },
  },
} as const satisfies SxProps<Theme>;

/**
 * Floating Paper surface for `@mui/x-date-pickers` desktop calendar and
 * multi-section digital clock poppers.
 *
 * Aligns the calendar grid, clock columns, and action bar with GM3 tokens and
 * suppresses native scrollbar tracks inside narrow 56px clock columns.
 */
export const datePickerPaperSx = {
  backgroundColor: fleetColors.surface,
  backgroundImage: 'none',
  border: borders.default,
  borderRadius: radii.md,
  boxShadow: elevations.card,
  overflow: 'hidden',
  '& .MuiPickersLayout-root': {
    backgroundColor: fleetColors.surface,
  },
  '& .MuiDateCalendar-root': {
    width: 304,
    height: 296,
    maxHeight: 296,
  },
  '& .MuiPickersCalendarHeader-root': {
    marginTop: '10px',
    marginBottom: '4px',
    paddingLeft: '18px',
    paddingRight: '10px',
  },
  '& .MuiPickersCalendarHeader-label': {
    fontFamily: fontFamilies.display,
    fontSize: fontSizes.body,
    fontWeight: 500,
    color: fleetColors.textPrimary,
  },
  '& .MuiDayCalendar-weekDayLabel': {
    fontFamily: fontFamilies.text,
    fontSize: fontSizes.caption,
    fontWeight: 500,
    color: fleetColors.textTertiary,
  },
  '& .MuiPickersDay-root': {
    fontFamily: fontFamilies.text,
    fontSize: fontSizes.label,
    color: fleetColors.textPrimary,
    borderRadius: radii.pill,
    '&:hover': {
      backgroundColor: fleetColors.surfaceTonal,
    },
    '&.MuiPickersDay-today:not(.Mui-selected)': {
      borderColor: fleetColors.primary,
      color: fleetColors.primary,
      fontWeight: 500,
    },
    '&.Mui-selected': {
      backgroundColor: fleetColors.primary,
      color: fleetColors.surface,
      fontWeight: 500,
      '&:hover, &:focus': {
        backgroundColor: fleetColors.primaryHover,
        color: fleetColors.surface,
      },
    },
  },
  '& .MuiMultiSectionDigitalClock-root': {
    borderBottom: 'none',
    borderLeft: borders.divider,
    maxHeight: 296,
  },
  '& .MuiMultiSectionDigitalClockSection-root': {
    width: 56,
    maxHeight: 296,
    padding: '4px 4px',
    scrollbarWidth: 'none',
    '&::-webkit-scrollbar': {
      display: 'none',
    },
    '&:not(:first-of-type)': {
      borderLeft: borders.divider,
    },
  },
  '& .MuiMultiSectionDigitalClockSection-item': {
    width: 44,
    minHeight: controlHeights.button,
    height: controlHeights.button,
    margin: '2px auto',
    padding: 0,
    borderRadius: radii.sm,
    fontFamily: fontFamilies.text,
    fontSize: fontSizes.label,
    color: fleetColors.textPrimary,
    justifyContent: 'center',
    '&:first-of-type': {
      marginTop: '4px',
    },
    '&:hover, &:focus': {
      backgroundColor: fleetColors.surfaceTonal,
    },
    '&.Mui-selected': {
      backgroundColor: fleetColors.primary,
      color: fleetColors.surface,
      fontWeight: 500,
      '&:hover, &:focus, &:active': {
        backgroundColor: fleetColors.primaryHover,
        color: fleetColors.surface,
      },
    },
  },
  '& .MuiPickersLayout-actionBar': {
    borderTop: borders.divider,
    padding: '6px 12px',
    minHeight: controlHeights.tab,
    '& .MuiButton-root': {
      fontFamily: fontFamilies.display,
      fontSize: fontSizes.label,
      fontWeight: 500,
      textTransform: 'none',
      color: fleetColors.primary,
      borderRadius: radii.pill,
      padding: '4px 16px',
      '&:hover': {
        backgroundColor: fleetColors.primaryContainer,
      },
    },
  },
} as const satisfies SxProps<Theme>;
