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
 * Fleet Console color palettes: the single source of truth for every color in
 * the light and dark themes.
 *
 * Each palette gives a concrete value to the same semantic roles. Components
 * never read this file: they use the tokens in `tokens.ts` (`fleetColors`,
 * `statusTones`, `toneColors`, `healthStateColors`, `avatarColors`), which are
 * CSS `var()` references. `FleetThemeProvider` writes the active palette as CSS
 * custom properties on `:root`. The light values double as the `var()`
 * fallbacks.
 *
 * To add a color: add a role to the interface below, give it a value in BOTH
 * `lightPalette` and `darkPalette`, then use it through `tokens.ts`.
 * `tokens.test.ts` fails if a role is missing a value or fails contrast.
 */

import { CSS_VAR_PREFIX, mix } from './color_math';
import { legacyScales, rawUnknownStateColor } from './colors';

export type FleetThemeId = 'light' | 'dark';

export type StatusTone =
  | 'success'
  | 'warning'
  | 'error'
  | 'info'
  | 'neutral'
  | 'special';

export interface StatusToneColors {
  /** Text and icon color. */
  fg: string;
  /** Pill fill. */
  bg: string;
  /** Leading dot / indicator color. */
  dot: string;
}

/** Semantic color roles. */
export interface FleetColorRoles {
  /** Page background behind cards. */
  canvas: string;
  /** Card, table, and header surface. */
  surface: string;
  /** Subtle tonal fill: row hover, tab/button hover, nested panels, tooltips. */
  surfaceTonal: string;
  /** Resting fill for data-table column header rows. */
  tableHeader: string;
  /** Hover fill for interactive icon buttons inside data-table column headers. */
  tableHeaderHover: string;
  /** Pill toolbar buttons and table pagination footers inside surface cards. */
  controlSurface: string;
  /** Full-bleed page headers and docked tab strips (e.g. Device details). */
  pageHeader: string;
  /** Background of the app shell (top header and side navigation). */
  shell: string;
  /** Default 1px border for cards, tables, inputs (GM3 outlineVariant). */
  border: string;
  /** Lighter 1px border for legacy panels and hairlines. */
  borderSubtle: string;
  /** Hairline divider between table rows and list items. */
  divider: string;
  /** Emphasized border for table headers and active boundaries. */
  borderEmphasis: string;
  /** Border of primary-tinted outlined controls. */
  borderPrimary: string;
  /** Border of any outlined control on hover. */
  borderHover: string;
  primary: string;
  primaryHover: string;
  /** Text and icons on a filled `primary` background. */
  onPrimary: string;
  /** Fill for selected/active tonal elements (filter chips, active tabs). */
  primaryContainer: string;
  primaryContainerHover: string;
  /** Text on `primaryContainer`. */
  onPrimaryContainer: string;
  /** Selected side-nav item fill, its hover fill, and its text/icon color. */
  navSelected: string;
  navSelectedHover: string;
  onNavSelected: string;
  /** Pale info banner and card fills (e.g. AI analysis summaries). */
  infoBanner: string;
  infoCard: string;
  /** Accent stripe and link color inside info banners. */
  infoAccent: string;
  /** Neutral grey panel (log links, code blocks). */
  panelMuted: string;
  /** Barely tinted card on the surface. */
  panelSubtle: string;
  /** Empty track behind progress and segmented bars. */
  track: string;
  /** Text on light tinted chips (`toneColors.*.container`). */
  onTint: string;
  /** Fill for the "unknown" device state chip. */
  unknownState: string;
  /** Text on `unknownState` chips. */
  onUnknownState: string;
  textPrimary: string;
  textSecondary: string;
  textTertiary: string;
  /** De-emphasized placeholder text, e.g. "(blank)" filter options. */
  textMuted: string;
  /** Text of options that are out of scope or not selectable. */
  textDisabled: string;
  /** Media overlay scrim for floating controls over photos (rgba allowed). */
  scrim: string;
  /** Foreground text and icons on `scrim`. */
  onScrim: string;
  /** Hover state layer inside `scrim` controls (rgba allowed). */
  scrimHover: string;
}

export type FleetColorRole = keyof FleetColorRoles;

/**
 * Color families for chips, indicators, avatars and chart series.
 * `success`/`error`/`warning`/`info`/`special`/`neutral` carry meaning; the
 * rest are decorative hues for categories.
 */
export type ToneName =
  | 'success'
  | 'error'
  | 'warning'
  | 'caution'
  | 'info'
  | 'special'
  | 'teal'
  | 'pink'
  | 'neutral';

/** Roles inside a tone, from the faintest fill to the strongest text. */
export interface ToneRoles {
  /** Faint tinted background (large areas, row highlights). */
  surface: string;
  /** Chip / badge fill. */
  container: string;
  /** Stronger fill: hover on a container, selected chip. */
  containerStrong: string;
  /** Tinted border. */
  border: string;
  /** Stronger tinted border, light chart fills. */
  borderStrong: string;
  /** Dots, bars, chart series, filled icons. */
  accent: string;
  /** Icons and text on the page surface. */
  main: string;
  /** Emphasized icons and text on the page surface. */
  mainStrong: string;
  /** Text on `container`. */
  onContainer: string;
  /** Strongest text on `container`. */
  onContainerStrong: string;
}

export type ToneRole = keyof ToneRoles;

export interface HealthStateColors {
  reserved: string;
  ready: string;
  needsRepair: string;
  repairFailed: string;
  needsDeploy: string;
  needsManualRepair: string;
  needsReplacement: string;
  registered: string;
  unknown: string;
  macroHealthy: string;
  macroActionRequired: string;
  macroAdministrative: string;
}

export interface AvatarColors {
  /** Saturated circle fills paired with `onFill` initials (same in light & dark). */
  readonly fills: readonly string[];
  /** Monogram initial color on avatar fills (`#ffffff` in both themes). */
  readonly onFill: string;
}

export interface FleetPalette {
  id: FleetThemeId;
  mode: 'light' | 'dark';
  colors: FleetColorRoles;
  status: Record<StatusTone, StatusToneColors>;
  tones: Record<ToneName, ToneRoles>;
  health: HealthStateColors;
  avatar: AvatarColors;
  elevations: { card: string; input: string };
}

/**
 * Non-inverting saturated avatar palette shared by both light and dark themes
 * (paired with `#ffffff` initials, matching Google Account avatars).
 */
const AVATAR_PALETTE: AvatarColors = {
  fills: [
    legacyScales.blue[700],
    legacyScales.green[700],
    legacyScales.purple[700],
    legacyScales.orange[700],
    legacyScales.red[700],
    legacyScales.cyan[800],
    legacyScales.pink[700],
    legacyScales.yellow[900],
    legacyScales.blue[900],
    legacyScales.orange[900],
  ],
  onFill: '#ffffff',
};

/** Which legacy hue scale backs each tone in the light theme. */
const TONE_HUES: Record<ToneName, keyof typeof legacyScales> = {
  success: 'green',
  error: 'red',
  warning: 'yellow',
  caution: 'orange',
  info: 'blue',
  special: 'purple',
  teal: 'cyan',
  pink: 'pink',
  neutral: 'grey',
};

/** Light tones are the original Material hue scales, step for step. */
function lightTone(tone: ToneName): ToneRoles {
  const s = legacyScales[TONE_HUES[tone]];
  return {
    surface: s[50],
    container: s[100],
    containerStrong: s[200],
    border: s[300],
    borderStrong: s[400],
    accent: s[500],
    main: s[600],
    mainStrong: s[700],
    onContainer: s[800],
    onContainerStrong: s[900],
  };
}

// ---------------------------------------------------------------------------
// Light
// ---------------------------------------------------------------------------

const lightColors: FleetColorRoles = {
  canvas: '#f8fafd',
  surface: '#ffffff',
  surfaceTonal: '#f3f6fc',
  tableHeader: '#f3f6fc',
  tableHeaderHover: 'rgba(0, 0, 0, 0.05)',
  controlSurface: '#f8fafd',
  pageHeader: '#ffffff',
  shell: '#ffffff',
  border: '#dadce0',
  borderSubtle: '#e0e0e0',
  divider: '#e0e2ec',
  borderEmphasis: '#c4c7c5',
  borderPrimary: '#c2d7f8',
  borderHover: '#8ab4f8',
  primary: '#0b57d0',
  primaryHover: '#0842a0',
  onPrimary: '#ffffff',
  primaryContainer: '#e8f0fe',
  primaryContainerHover: '#d2e3fc',
  onPrimaryContainer: '#0b57d0',
  navSelected: legacyScales.blue[50],
  navSelectedHover: legacyScales.blue[100],
  onNavSelected: legacyScales.blue[800],
  infoBanner: '#eef6fc',
  infoCard: '#f0f7ff',
  infoAccent: '#1976d2',
  panelMuted: '#f5f5f5',
  panelSubtle: '#f9f9f9',
  track: '#eeeeee',
  onTint: 'rgba(0, 0, 0, 0.87)',
  unknownState: rawUnknownStateColor,
  onUnknownState: '#005b4f',
  textPrimary: '#1f1f1f',
  textSecondary: '#444746',
  textTertiary: '#5f6368',
  textMuted: '#616161',
  textDisabled: '#9e9e9e',
  scrim: 'rgba(31, 31, 31, 0.76)',
  onScrim: '#ffffff',
  scrimHover: 'rgba(255, 255, 255, 0.14)',
};

export const lightPalette: FleetPalette = {
  id: 'light',
  mode: 'light',
  colors: lightColors,
  status: {
    success: { fg: '#137333', bg: '#e6f4ea', dot: '#1e8e3e' },
    warning: { fg: '#9e5400', bg: '#fef7e0', dot: '#f9ab00' },
    error: { fg: '#c5221f', bg: '#fce8e6', dot: '#d93025' },
    info: {
      fg: lightColors.primary,
      bg: lightColors.primaryContainer,
      dot: '#1a73e8',
    },
    neutral: { fg: lightColors.textSecondary, bg: '#f1f3f4', dot: '#80868b' },
    special: { fg: '#681da8', bg: '#f3e8fd', dot: '#9334e6' },
  },
  tones: {
    success: lightTone('success'),
    error: lightTone('error'),
    warning: lightTone('warning'),
    caution: { ...lightTone('caution'), onContainer: '#8c4700' },
    info: lightTone('info'),
    special: lightTone('special'),
    teal: lightTone('teal'),
    pink: lightTone('pink'),
    neutral: lightTone('neutral'),
  },
  health: {
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
  },
  avatar: AVATAR_PALETTE,
  elevations: {
    card: '0 1px 3px rgba(15, 23, 42, 0.04), 0 1px 2px rgba(15, 23, 42, 0.02)',
    input: '0 1px 2px rgba(15, 23, 42, 0.03)',
  },
};

// ---------------------------------------------------------------------------
// Dark (GM3 Flipped: #1e1f20 canvas & page header, #131314 solid surfaces & shell)
// ---------------------------------------------------------------------------

const darkColors: FleetColorRoles = {
  canvas: '#1e1f20',
  surface: '#131314',
  surfaceTonal: '#282a2c',
  tableHeader: '#1c1d1e',
  tableHeaderHover: 'rgba(255, 255, 255, 0.08)',
  controlSurface: '#1b1b1b',
  pageHeader: '#1e1f20',
  shell: '#131314',
  border: '#444746',
  borderSubtle: '#3a3c3e',
  divider: '#333537',
  borderEmphasis: '#5e6164',
  borderPrimary: '#284777',
  borderHover: '#7cacf8',
  primary: '#a8c7fa',
  primaryHover: '#d3e3fd',
  onPrimary: '#062e6f',
  primaryContainer: '#004a77',
  primaryContainerHover: '#0b5a8e',
  onPrimaryContainer: '#d3e3fd',
  navSelected: '#004a77',
  navSelectedHover: '#0b5a8e',
  onNavSelected: '#d3e3fd',
  infoBanner: '#1a2332',
  infoCard: '#1b222c',
  infoAccent: '#a8c7fa',
  panelMuted: '#1e1f20',
  panelSubtle: '#1b1b1b',
  track: '#333537',
  onTint: '#e3e3e3',
  unknownState: '#1f4a41',
  onUnknownState: '#a6ebd9',
  textPrimary: '#e3e3e3',
  textSecondary: '#c4c7c5',
  textTertiary: '#a3a7ab',
  textMuted: '#a3a7ab',
  textDisabled: '#7c7f82',
  scrim: 'rgba(0, 0, 0, 0.72)',
  onScrim: '#ffffff',
  scrimHover: 'rgba(255, 255, 255, 0.14)',
};

const darkStatus: Record<StatusTone, StatusToneColors> = {
  success: { fg: '#6dd58c', bg: '#0f3d1e', dot: '#37be5f' },
  warning: { fg: '#ffb95c', bg: '#46300a', dot: '#f9ab00' },
  error: { fg: '#ffb4ab', bg: '#5c1512', dot: '#f2726a' },
  info: { fg: '#d3e3fd', bg: '#004a77', dot: '#7cacf8' },
  neutral: { fg: '#c4c7c5', bg: '#333537', dot: '#8e918f' },
  special: { fg: '#e5b8ff', bg: '#43195f', dot: '#c58af9' },
};

/** A dark tone built around a container fill, its text color and an accent. */
function darkTone(
  accent: string,
  container: string,
  onContainer: string,
): ToneRoles {
  const s = darkColors.surface;
  return {
    surface: mix(accent, s, 0.1),
    container,
    containerStrong: mix(accent, container, 0.25),
    border: mix(accent, s, 0.45),
    borderStrong: mix(accent, s, 0.7),
    accent,
    main: onContainer,
    mainStrong: onContainer,
    onContainer,
    onContainerStrong: mix(onContainer, '#ffffff', 0.4),
  };
}

export const darkPalette: FleetPalette = {
  id: 'dark',
  mode: 'dark',
  colors: darkColors,
  status: darkStatus,
  tones: {
    success: darkTone('#37be5f', '#0f3d1e', '#6dd58c'),
    error: darkTone('#f2726a', '#5c1512', '#ffb4ab'),
    warning: darkTone('#f9ab00', '#46300a', '#ffc96b'),
    caution: darkTone('#fa903e', '#4a2509', '#ffc69e'),
    info: darkTone('#7cacf8', '#0f3a66', '#a8c7fa'),
    special: darkTone('#c58af9', '#43195f', '#e5b8ff'),
    teal: darkTone('#4ecde6', '#0b3b44', '#8be4f4'),
    pink: darkTone('#ff6fb4', '#5a1838', '#ffb0d6'),
    neutral: {
      surface: '#1b1b1b',
      container: '#333537',
      containerStrong: '#3c3f41',
      border: darkColors.border,
      borderStrong: darkColors.borderEmphasis,
      accent: '#8e918f',
      main: darkColors.textTertiary,
      mainStrong: darkColors.textSecondary,
      onContainer: darkColors.textSecondary,
      onContainerStrong: darkColors.textPrimary,
    },
  },
  health: {
    reserved: '#2e7d32',
    ready: '#4caf50',
    needsRepair: '#a5d6a7',
    repairFailed: '#a5d6a7',
    needsDeploy: '#ef5350',
    needsManualRepair: '#ef5350',
    needsReplacement: '#ef5350',
    registered: '#9e9e9e',
    unknown: '#616161',
    macroHealthy: '#4caf50',
    macroActionRequired: '#ef5350',
    macroAdministrative: '#9e9e9e',
  },
  avatar: AVATAR_PALETTE,
  elevations: { card: 'none', input: 'none' },
};

export const FLEET_PALETTES: Record<FleetThemeId, FleetPalette> = {
  light: lightPalette,
  dark: darkPalette,
};

// ---------------------------------------------------------------------------
// CSS custom properties
// ---------------------------------------------------------------------------

const kebab = (s: string) => s.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);

/** CSS custom property names (without the `--fc-` prefix). */
export const cssVarNames = {
  color: (role: FleetColorRole) => kebab(role),
  status: (tone: StatusTone, part: keyof StatusToneColors) =>
    `status-${tone}-${part}`,
  tone: (tone: ToneName, role: ToneRole) => `tone-${tone}-${kebab(role)}`,
  health: (key: keyof HealthStateColors) => `health-${kebab(key)}`,
  avatarFill: (index: number) => `avatar-fill-${index}`,
  avatarOnFill: 'avatar-on-fill',
  elevation: (key: 'card' | 'input') => `elevation-${key}`,
};

function entries<T extends object>(o: T): [keyof T, T[keyof T]][] {
  return Object.entries(o) as [keyof T, T[keyof T]][];
}

/** All Fleet CSS custom properties for a palette, keyed by full name. */
export function fleetThemeCssVars(p: FleetPalette): Record<string, string> {
  const vars: Record<string, string> = {};
  const set = (name: string, value: string) => {
    vars[`${CSS_VAR_PREFIX}${name}`] = value;
  };
  for (const [role, value] of entries(p.colors)) {
    set(cssVarNames.color(role), value);
  }
  for (const [tone, parts] of entries(p.status)) {
    for (const [part, value] of entries(parts)) {
      set(cssVarNames.status(tone, part), value);
    }
  }
  for (const [tone, roles] of entries(p.tones)) {
    for (const [role, value] of entries(roles)) {
      set(cssVarNames.tone(tone, role), value);
    }
  }
  for (const [key, value] of entries(p.health)) {
    set(cssVarNames.health(key), value);
  }
  p.avatar.fills.forEach((value, index) => {
    set(cssVarNames.avatarFill(index), value);
  });
  set(cssVarNames.avatarOnFill, p.avatar.onFill);
  set(cssVarNames.elevation('card'), p.elevations.card);
  set(cssVarNames.elevation('input'), p.elevations.input);
  return vars;
}

/**
 * Overrides for the shared LUCI UI CSS custom properties
 * (src/common/styles/style.css) read by shared components and plain `<a>` /
 * `<hr>` elements, so they follow the active Fleet palette.
 */
export function luciCommonCssVars(p: FleetPalette): Record<string, string> {
  const c = p.colors;
  const s = p.status;
  return {
    '--default-text-color': c.textPrimary,
    '--light-text-color': c.textSecondary,
    '--greyed-out-text-color': c.textTertiary,
    '--active-text-color': c.primary,
    '--active-color': c.primary,
    '--light-active-color': c.primaryContainer,
    '--block-background-color': c.surfaceTonal,
    '--divider-color': c.divider,
    '--success-bg-color': s.success.bg,
    '--failure-bg-color': s.error.bg,
    '--started-bg-color': s.warning.bg,
    '--scheduled-bg-color': s.neutral.bg,
    '--critical-failure-bg-color': s.special.bg,
    '--canceled-bg-color': s.info.bg,
    '--warning-bg-color': s.warning.bg,
    '--gm3-color-surface': c.surface,
    '--gm3-color-outline-variant': c.border,
    '--gm3-color-on-surface-variant': c.textSecondary,
    '--gm3-color-on-surface': c.textSecondary,
    '--gm3-color-primary': c.primary,
    '--gm3-color-on-surface-strong': c.textPrimary,
    '--gm3-color-on-surface-medium': c.textSecondary,
    '--gm3-color-on-surface-light': c.textTertiary,
    '--gm3-color-error': s.error.fg,
    '--gm3-color-error-container': s.error.bg,
    '--gm3-color-success': s.success.fg,
    '--gm3-color-success-container': s.success.bg,
    '--gm3-color-warning': s.warning.fg,
    '--gm3-color-warning-dark': s.warning.fg,
    '--gm3-color-warning-container': s.warning.bg,
    '--gm3-color-primary-container': c.primaryContainer,
    '--gm3-color-surface-container': c.surfaceTonal,
    '--gm3-color-surface-container-low': c.panelSubtle,
    '--gm3-color-on-primary-container': c.onPrimaryContainer,
    '--gm3-color-on-error-container': s.error.fg,
    '--gm3-color-on-success-container': s.success.fg,
    '--gm3-color-on-warning-container': s.warning.fg,
    '--gm3-color-on-info-container': s.info.fg,
  };
}
