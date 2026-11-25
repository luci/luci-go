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

import { Linter } from 'eslint';

import fconPlugin from './index';

// jsdom does not expose Node's structuredClone, which ESLint 9's flat config
// schema calls when normalizing rule options.
if (typeof globalThis.structuredClone !== 'function') {
  globalThis.structuredClone = <T>(value: T): T =>
    JSON.parse(JSON.stringify(value)) as T;
}

const linter = new Linter({ configType: 'flat' });

function lintCode(code: string) {
  return linter.verify(code, [
    {
      files: ['**/*.js'],
      languageOptions: {
        ecmaVersion: 2022,
        sourceType: 'module',
        parserOptions: { ecmaFeatures: { jsx: true } },
      },
      plugins: { 'fcon-rules': fconPlugin },
      rules: { 'fcon-rules/no-hardcoded-colors': 'error' },
    },
  ]);
}

describe('fcon-rules/no-hardcoded-colors', () => {
  it.each([
    'const sx = { color: fleetColors.textPrimary };',
    'const url = "https://example.com/#123456";',
    'const url = `https://example.com/${id}#123456`;',
    'const svgRef = "url(#abc)";',
    'const text = "See CL #123456 and Bug #abc";',
    'const sx = { color: "inherit", backgroundColor: "transparent" };',
    'const sx = { color: status === "red" ? fleetColors.primary : fleetColors.textPrimary };',
  ])('allows valid code: %s', (code) => {
    expect(lintCode(code)).toEqual([]);
  });

  it.each([
    ['hex literal', 'const sx = { color: "#1976d2" };', 'rawColor'],
    ['short hex', 'const sx = { border: "1px solid #fff" };', 'rawColor'],
    [
      'rgba literal',
      'const sx = { bgcolor: "rgba(0, 0, 0, 0.5)" };',
      'rawColor',
    ],
    ['hsl literal', 'const sx = { color: "hsl(210, 50%, 50%)" };', 'rawColor'],
    ['named color prop', 'const sx = { color: "white" };', 'rawColor'],
    [
      'named color in ternary',
      'const sx = { color: ok ? "green" : "red" };',
      'rawColor',
    ],
    [
      'named color in logical &&',
      'const sx = { color: ok && "red" };',
      'rawColor',
    ],
    [
      'named color in logical ||',
      'const sx = { color: custom || "white" };',
      'rawColor',
    ],
    [
      'named color in JSX ternary',
      'const el = <Box color={ok ? "green" : "red"} />;',
      'rawColor',
    ],
    ['mui grey string', 'const sx = { color: "grey.500" };', 'muiGrey'],
    ['mui common string', 'const sx = { color: "common.white" };', 'muiGrey'],
    ['theme.palette.grey', 'const c = theme.palette.grey[400];', 'muiGrey'],
    ['destructured palette.grey', 'const c = palette.grey[400];', 'muiGrey'],
    ['palette.common', 'const c = theme.palette.common.white;', 'muiGrey'],
    [
      'mui colors import',
      'import { blue } from "@mui/material/colors";',
      'forbiddenImport',
    ],
    [
      'mui material colors named import',
      'import { colors } from "@mui/material";',
      'forbiddenImport',
    ],
    [
      'fleet theme palettes import',
      'import { darkPalette } from "@/fleet/theme/palettes";',
      'forbiddenImport',
    ],
    [
      'alpha on CSS var token',
      'const c = alpha(fleetColors.primary, 0.1);',
      'tokenColorMath',
    ],
  ])('flags %s', (_label, code, expectedMessageId) => {
    const messages = lintCode(code);
    expect(messages.length).toBeGreaterThan(0);
    expect(messages[0].messageId).toBe(expectedMessageId);
  });
});
