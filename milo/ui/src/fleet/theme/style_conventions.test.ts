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
 * theme/README.md). Legacy code still has some literals; these baselines stop
 * the count from growing. When you remove literals, LOWER the baseline. Never
 * raise it: use a token, recipe, or theme variant instead.
 */
const BASELINE = {
  rgba: 33,
  radius: 80,
  hex: 13,
  shadow: 7,
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
};

function listSourceFiles(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return listSourceFiles(full);
    return /\.tsx?$/.test(entry.name) ? [full] : [];
  });
}

function countLiterals() {
  const counts = { radius: 0, hex: 0, shadow: 0, rgba: 0 };
  const offenders: Record<keyof typeof BASELINE, string[]> = {
    radius: [],
    hex: [],
    shadow: [],
    rgba: [],
  };
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
