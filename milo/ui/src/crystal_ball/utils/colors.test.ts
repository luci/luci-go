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

import { generateColor, getSafeCssColor, isSafeCssColor } from './colors';

describe('isSafeCssColor', () => {
  test.each([
    // UI color picker (<input type="color">) and generateColor output.
    '#3b26d9',
    // GenerateDashboardState prompt palette (uppercase hex).
    '#4285F4',
    '#fff',
    '#ffff',
    '#ff000080',
    'red',
    'RebeccaPurple',
    'transparent',
    'rgb(255, 0, 0)',
    'rgba(255, 0, 0, 0.5)',
    'rgb(255 0 0 / 50%)',
    'hsl(120, 50%, 50%)',
    'hsl(120deg 50% 50% / 0.5)',
    'hsla(0.5turn, 50%, 50%, 1)',
  ])('accepts %s', (color) => {
    expect(isSafeCssColor(color)).toBe(true);
  });

  test.each([
    '',
    ' red',
    'red ',
    '#ff',
    '#fffff',
    '#gggggg',
    'red;',
    'red}*{position:fixed}',
    'red}*{position:fixed;inset:0;background:url(//example.com/beacon);opacity:.01}.x{--x:',
    'url(//example.com/beacon)',
    'var(--x)',
    'rgb(0,0,0);background:url(x)',
    'rgb(0,0,0)}*{color:red',
    'rgb(calc(1),0,0)',
    'expression(alert(1))',
    'red\n}',
  ])('rejects %j', (color) => {
    expect(isSafeCssColor(color)).toBe(false);
  });

  test('rejects non-strings', () => {
    expect(isSafeCssColor(undefined)).toBe(false);
    expect(isSafeCssColor(null)).toBe(false);
    expect(isSafeCssColor(123)).toBe(false);
  });

  test('accepts every generated palette color', () => {
    for (let i = 0; i < 100; i++) {
      expect(isSafeCssColor(generateColor(i))).toBe(true);
    }
  });
});

describe('getSafeCssColor', () => {
  test('returns safe colors unchanged', () => {
    expect(getSafeCssColor('#4285F4')).toBe('#4285F4');
  });

  test('returns undefined for unsafe colors', () => {
    expect(getSafeCssColor('red}*{position:fixed}')).toBeUndefined();
    expect(getSafeCssColor(undefined)).toBeUndefined();
  });
});
