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

import * as fs from 'fs';
import * as path from 'path';

/**
 * Ratchet on raw style literals in Fleet Console components.
 *
 * All colors, radii, and shadows must come from `src/fleet/theme/` (see
 * theme/README.md). The zeroed color baselines serve as a fast regex backstop
 * alongside the compile-time `fcon-rules/no-hardcoded-colors` ESLint rule.
 * When you remove legacy `radius` literals, LOWER the baseline. Never raise it.
 */
const BASELINE = {
  rgba: 0,
  radius: 48,
  hex: 0,
  shadow: 0,
  hsl: 0,
  namedColor: 0,
  muiGrey: 0,
};

const FLEET_ROOT = path.resolve(__dirname, '..');

/** Paths (relative to src/fleet) allowed to hold raw values. */
const EXEMPT = [
  /^theme\//,
  /^testing_tools\//,
  /^\.agents\//,
  /\.test\.tsx?$/,
  /\.stories\.tsx?$/,
  /(^|\/)mocks?\//,
];

const PATTERNS: Record<keyof typeof BASELINE, RegExp> = {
  // borderRadius: '8px' | borderRadius: 2 | border-radius: 4px ('50%' and 0 are fine).
  radius: /border-?radius:\s*['"`]?(?:[1-9]\d*(?:\.\d+)?|0\.\d+)(?![\d.%])/gi,
  // '#1f1f1f' in JS, or `color: #1f1f1f;` in CSS template literals.
  hex: /(['"`]#[0-9a-f]{3,8}\b)|(:\s*#[0-9a-f]{3,8}\b)/gi,
  // rgba(15, 23, 42, 0.04) and friends: use fleetColors/elevations.
  rgba: /rgba?\(\s*\d/gi,
  // boxShadow: '0 1px 2px ...' (theme shadow indexes like boxShadow: 1 are fine).
  shadow: /box-?shadow:\s*['"`](?!none|inherit|unset|initial)[^'"`]*\d+px/gi,
  // hsl(217, 89%, 61%): use toneColors / fleetColors.
  hsl: /hsla?\(\s*\d/gi,
  // color: 'white', bgcolor: 'black', fill="red" ...: use fleetColors.
  namedColor:
    /\b(?:color|backgroundColor|background|bgcolor|borderColor|fill|stroke)[:=]\s*\{?\s*['"](?:white|black|red|green|blue|gr[ae]y|orange|yellow|purple|pink)['"]/gi,
  // theme.palette.grey[400] or sx 'grey.200': MUI greys don't follow the theme.
  muiGrey: /palette\.grey\[|['"]grey\.\d+['"]/g,
};

function listSourceFiles(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return listSourceFiles(full);
    return /\.tsx?$/.test(entry.name) ? [full] : [];
  });
}

function countLiterals() {
  const counts = Object.fromEntries(
    Object.keys(BASELINE).map((k) => [k, 0]),
  ) as Record<keyof typeof BASELINE, number>;
  const offenders = Object.fromEntries(
    Object.keys(BASELINE).map((k) => [k, [] as string[]]),
  ) as Record<keyof typeof BASELINE, string[]>;
  for (const file of listSourceFiles(FLEET_ROOT)) {
    const rel = path.relative(FLEET_ROOT, file).split(path.sep).join('/');
    if (EXEMPT.some((re) => re.test(rel))) continue;
    const text = fs.readFileSync(file, 'utf8');
    for (const key of Object.keys(PATTERNS) as (keyof typeof BASELINE)[]) {
      const n = text.match(PATTERNS[key])?.length ?? 0;
      if (n > 0) {
        counts[key] += n;
        offenders[key].push(`${rel} (${n})`);
      }
    }
  }
  return { counts, offenders };
}

describe('Fleet styling conventions', () => {
  const { counts, offenders } = countLiterals();

  it.each(Object.keys(BASELINE) as (keyof typeof BASELINE)[])(
    'does not add raw %s literals outside src/fleet/theme',
    (key) => {
      if (counts[key] > BASELINE[key]) {
        throw new Error(
          `Found ${counts[key]} raw ${key} literals (baseline ${BASELINE[key]}). ` +
            'Use tokens/recipes/variants from src/fleet/theme/ (see ' +
            `theme/README.md). Files:\n  ${offenders[key].join('\n  ')}`,
        );
      }
      expect(counts[key]).toBeLessThanOrEqual(BASELINE[key]);
    },
  );

  it('keeps baselines tight so the ratchet only moves down', () => {
    for (const key of Object.keys(BASELINE) as (keyof typeof BASELINE)[]) {
      if (counts[key] < BASELINE[key]) {
        throw new Error(
          `Nice, you removed raw ${key} literals. Lower BASELINE.${key} ` +
            `from ${BASELINE[key]} to ${counts[key]} in this file.`,
        );
      }
      expect(counts[key]).toBe(BASELINE[key]);
    }
  });
});

/**
 * Light/dark switching lives only in src/fleet/theme. Components consume
 * theme-aware tokens and never branch on the active theme or reach for
 * static color scales. These are hard failures, not a ratchet.
 */
const THEME_LOGIC_ALLOWED = [/^theme\//, /\.test\.tsx?$/, /^\.agents\//];

const FORBIDDEN_THEME_LOGIC: Record<string, RegExp> = {
  'raw hue scales (use toneColors/fleetColors from @/fleet/theme/tokens)':
    /from ['"](?:@\/fleet\/theme|\.\.?(?:\/\.\.)*)\/colors['"]/,
  'static MUI colors (use theme tokens)':
    /from ['"]@mui\/material\/colors['"]|import\s+\{[^}]*\bcolors\b[^}]*\}\s+from\s+['"]@mui\/material['"]/,
  'theme branching (palette.mode / isDark / color-scheme)':
    /palette\.mode|\bisDark\b|prefers-color-scheme|useColorScheme|data-fleet-theme/,
};

/** Files that may read the theme choice (the theme picker). */
const THEME_CHOICE_ALLOWED = [/^layouts\/settings_menu\.tsx$/];

describe('Fleet theme boundary', () => {
  const files = listSourceFiles(FLEET_ROOT)
    .map((file) => ({
      file,
      rel: path.relative(FLEET_ROOT, file).split(path.sep).join('/'),
    }))
    .filter(({ rel }) => !THEME_LOGIC_ALLOWED.some((re) => re.test(rel)));

  it.each(Object.entries(FORBIDDEN_THEME_LOGIC))(
    'has no %s outside src/fleet/theme',
    (_label, re) => {
      const offenders = files
        .filter(({ file }) => re.test(fs.readFileSync(file, 'utf8')))
        .map(({ rel }) => rel);
      expect(offenders).toEqual([]);
    },
  );

  it('only reads the theme choice in the theme picker', () => {
    const offenders = files
      .filter(({ rel }) => !THEME_CHOICE_ALLOWED.some((re) => re.test(rel)))
      .filter(({ file }) =>
        /useFleetTheme\(/.test(fs.readFileSync(file, 'utf8')),
      )
      .map(({ rel }) => rel);
    expect(offenders).toEqual([]);
  });
});
