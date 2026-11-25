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
 * Fleet Console design tokens: the single source of truth for every radius,
 * surface color, border, shadow, font size, and control height in the Fleet UI.
 *
 * Rules (see src/fleet/GEMINI.md, "Styling conventions"):
 * - Never write a raw hex color, `borderRadius`, `boxShadow`, or `fontSize`
 *   literal in a component. Import a token from here, or better, use a recipe
 *   from `@/fleet/theme/recipes` or a theme variant (`<Button variant="toolbar">`,
 *   `<Chip variant="filter">`, `<StatusPill>`).
 * - Need a value that does not exist? Add it here (with a comment on when to
 *   use it) instead of inlining a near-duplicate.
 */

import { cssVar } from './color_math';
import {
  cssVarNames,
  FleetColorRole,
  HealthStateColors,
  lightPalette,
  StatusTone,
  StatusToneColors,
  ToneName,
  ToneRole,
  ToneRoles,
} from './palettes';

/**
 * Corner radius scale. Only three steps, on purpose.
 */
export const radii = {
  /** Small inner elements: code snippets, tooltips, menu items, inputs. */
  sm: '6px',
  /** Every surface container: cards, tables, accordions, menus, dialogs. */
  md: '12px',
  /** Anything pill-shaped: buttons, chips, status pills, search bars. */
  pill: '9999px',
} as const;

/**
 * Semantic color roles, aligned with Google Material 3.
 *
 * Each value is a CSS `var()` that follows the active Fleet theme (light or
 * dark in `palettes.ts`) and falls back to the light value. Use them anywhere CSS accepts a color. Never pass them to MUI's `alpha()`,
 * `darken()`, or `lighten()`: read `theme.palette` for computed colors instead.
 * See `palettes.ts` (`FleetColorRoles`) for what each role is for.
 */
export const fleetColors = Object.fromEntries(
  (Object.keys(lightPalette.colors) as FleetColorRole[]).map((role) => [
    role,
    cssVar(cssVarNames.color(role), lightPalette.colors[role]),
  ]),
) as Readonly<Record<FleetColorRole, string>>;

export const borders = {
  default: `1px solid ${fleetColors.border}`,
  divider: `1px solid ${fleetColors.divider}`,
  emphasis: `1px solid ${fleetColors.borderEmphasis}`,
} as const;

export const elevations = {
  /** Resting card. Barely visible; the border does most of the work. */
  card: cssVar(cssVarNames.elevation('card'), lightPalette.elevations.card),
  /** Inputs that sit directly on the canvas. */
  input: cssVar(cssVarNames.elevation('input'), lightPalette.elevations.input),
} as const;

export const fontFamilies = {
  /** Headings, buttons, and tab labels. */
  display: "'Google Sans', 'Roboto', sans-serif",
  /** Body copy, table cells, chips. */
  text: "'Google Sans Text', 'Google Sans', 'Roboto', sans-serif",
  mono: "'Roboto Mono', monospace",
} as const;

/**
 * Font sizes in px. Prefer `<Typography variant=...>`; use these only inside
 * recipes or when a MUI component has no typography variant hook.
 */
export const fontSizes = {
  /** Captions, status pills, helper text. */
  caption: 12,
  /** Controls: buttons, chips, tabs, table headers and cells. */
  label: 13,
  /** Default body text. */
  body: 14,
  /** Card and section titles. */
  title: 16,
  /** Page titles. */
  headline: 22,
  /** Big KPI numbers in scorecards. */
  display: 28,
} as const;

/** Heights in px for interactive controls. */
export const controlHeights = {
  /** Status pills and filter chips. */
  chip: 24,
  /** Status pills in compact table density. */
  chipCompact: 22,
  /** Toolbar and header action buttons, icon buttons. */
  button: 32,
  /** Search bars and text inputs. */
  input: 40,
  /** Docked tab strip under a page header. */
  tab: 44,
} as const;

/** Icon size (px) inside 32px controls. */
export const iconSizes = {
  control: 16,
  inline: 18,
  /** Platform logos in pickers and selectors. */
  logo: 20,
} as const;

/**
 * Semantic status tones for `<StatusPill>` and any colored state indicator.
 * Each tone has an accessible foreground on its tinted background (`fg`, `bg`,
 * `dot`).
 */
export type { StatusTone, ToneName, ToneRole };

export const statusTones = Object.fromEntries(
  (Object.keys(lightPalette.status) as StatusTone[]).map((tone) => {
    const light = lightPalette.status[tone];
    const part = (key: keyof StatusToneColors) =>
      cssVar(cssVarNames.status(tone, key), light[key]);
    return [tone, { fg: part('fg'), bg: part('bg'), dot: part('dot') }];
  }),
) as Readonly<Record<StatusTone, StatusToneColors>>;

/**
 * Spectrum color tokens for ChromeOS Fleet Health state breakdown and continuous progress bar.
 * Harmonious spectrum representing:
 * - Operable capacity (green spectrum): reserved (darkest) > ready > needsRepair/repairFailed.
 * - Action required / broken (red spectrum): needsDeploy, needsManualRepair, needsReplacement.
 * - Administrative / inventory (grey spectrum): registered, unknown.
 * Light values live in `palettes.ts`; dark themes lift lightness.
 */
export const healthStateColors = Object.fromEntries(
  (Object.keys(lightPalette.health) as (keyof HealthStateColors)[]).map(
    (key) => [key, cssVar(cssVarNames.health(key), lightPalette.health[key])],
  ),
) as Readonly<Record<keyof HealthStateColors, string>>;

/**
 * Color families for chips, indicators and chart series, e.g.
 * `toneColors.success.container` (chip fill) with
 * `toneColors.success.onContainer` (chip text), or `toneColors.info.accent`
 * (chart series). Prefer `statusTones` / `<StatusPill>` for task and device
 * states; use tones when you need more steps or a decorative hue. See
 * `ToneRoles` in `palettes.ts` for what each role is for.
 */
export const toneColors = Object.fromEntries(
  (Object.keys(lightPalette.tones) as ToneName[]).map((tone) => {
    const light = lightPalette.tones[tone];
    return [
      tone,
      Object.fromEntries(
        (Object.keys(light) as ToneRole[]).map((role) => [
          role,
          cssVar(cssVarNames.tone(tone, role), light[role]),
        ]),
      ),
    ];
  }),
) as Readonly<Record<ToneName, Readonly<ToneRoles>>>;

/**
 * Non-inverting user avatar circle fills and monogram initial color.
 * Avatars stay saturated with white initials in both light and dark themes
 * (matching Google Account avatars) instead of inverting to pastel fills.
 */
export const avatarColors = {
  fills: lightPalette.avatar.fills.map((value, index) =>
    cssVar(cssVarNames.avatarFill(index), value),
  ) as readonly string[],
  onFill: cssVar(cssVarNames.avatarOnFill, lightPalette.avatar.onFill),
} as const;

/** A fully transparent color (e.g. "no chip fill"). */
export const transparent = 'transparent';
