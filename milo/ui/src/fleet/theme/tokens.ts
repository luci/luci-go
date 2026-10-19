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
 */
export const fleetColors = {
  /** Page background behind cards. */
  canvas: '#f8fafd',
  /** Card, table, and header surface. */
  surface: '#ffffff',
  /** Subtle tonal fill: table headers, row hover, nested panels. */
  surfaceTonal: '#f3f6fc',

  /** Default 1px border for cards, tables, inputs (GM3 outlineVariant). */
  border: '#dadce0',
  /** Hairline divider between table rows and list items (High contrast). */
  divider: '#e0e2ec',
  /** Emphasized border for table headers and active boundaries. */
  borderEmphasis: '#c4c7c5',
  /** Border of primary-tinted outlined controls. */
  borderPrimary: '#c2d7f8',
  /** Border of any outlined control on hover. */
  borderHover: '#8ab4f8',

  primary: '#0b57d0',
  primaryHover: '#0842a0',
  /** Fill for selected/active tonal elements (filter chips, active tabs). */
  primaryContainer: '#e8f0fe',
  primaryContainerHover: '#d2e3fc',

  textPrimary: '#1f1f1f',
  textSecondary: '#444746',
  textTertiary: '#5f6368',
} as const;

export const borders = {
  default: `1px solid ${fleetColors.border}`,
  divider: `1px solid ${fleetColors.divider}`,
  emphasis: `1px solid ${fleetColors.borderEmphasis}`,
} as const;

export const elevations = {
  /** Resting card. Barely visible; the border does most of the work. */
  card: '0 1px 3px rgba(15, 23, 42, 0.04), 0 1px 2px rgba(15, 23, 42, 0.02)',
  /** Inputs that sit directly on the canvas. */
  input: '0 1px 2px rgba(15, 23, 42, 0.03)',
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
 * Each tone has an accessible foreground on its tinted background.
 */
export type StatusTone =
  | 'success'
  | 'warning'
  | 'error'
  | 'info'
  | 'neutral'
  | 'special';

export const statusTones: Record<
  StatusTone,
  { fg: string; bg: string; dot: string }
> = {
  success: { fg: '#137333', bg: '#e6f4ea', dot: '#1e8e3e' },
  warning: { fg: '#9e5400', bg: '#fef7e0', dot: '#f9ab00' },
  error: { fg: '#c5221f', bg: '#fce8e6', dot: '#d93025' },
  info: {
    fg: fleetColors.primary,
    bg: fleetColors.primaryContainer,
    dot: '#1a73e8',
  },
  neutral: { fg: fleetColors.textSecondary, bg: '#f1f3f4', dot: '#80868b' },
  special: { fg: '#681da8', bg: '#f3e8fd', dot: '#9334e6' },
};

/**
 * Spectrum color tokens for ChromeOS Fleet Health state breakdown and continuous progress bar.
 * Harmonious spectrum representing:
 * - Operable capacity (green spectrum):
 *   - reserved: dark green (#1b5e20)
 *   - ready: medium green (#43a047)
 *   - needsRepair: light green (#81c784)
 *   - repairFailed: light green (#81c784)
 * - Action required / broken (red spectrum):
 *   - needsDeploy: red (#d32f2f)
 *   - needsManualRepair: red (#d32f2f)
 *   - needsReplacement: red (#d32f2f)
 * - Administrative / inventory (grey spectrum):
 *   - registered: grey (#757575)
 *   - unknown: grey (#9e9e9e)
 */
export const healthStateColors = {
  reserved: '#1b5e20',
  ready: '#43a047',
  needsRepair: '#81c784',
  repairFailed: '#81c784',
  needsDeploy: '#d32f2f',
  needsManualRepair: '#d32f2f',
  needsReplacement: '#d32f2f',
  registered: '#757575',
  unknown: '#9e9e9e',
  macroHealthy: '#2e7d32',
  macroActionRequired: '#d32f2f',
  macroAdministrative: '#757575',
} as const;
