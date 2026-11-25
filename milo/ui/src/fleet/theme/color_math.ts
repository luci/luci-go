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
 * Tiny color helpers used to derive the dark-theme palettes (legacy hue scales,
 * tinted containers) and to check WCAG contrast in tests. Supports `#rgb`,
 * `#rrggbb`, and `hsl(h, s%, l%)` inputs only.
 */

export type Rgb = readonly [number, number, number];

/** Prefix of every Fleet Console theme CSS custom property. */
export const CSS_VAR_PREFIX = '--fc-';

/**
 * A CSS `var()` reference to a Fleet theme custom property, falling back to the
 * light-theme value so components render unchanged when no theme vars are set
 * (unit tests, Storybook, the prototype app).
 */
export function cssVar(name: string, fallback: string): string {
  return `var(${CSS_VAR_PREFIX}${name}, ${fallback})`;
}

function hslToRgb(h: number, s: number, l: number): Rgb {
  const sat = s / 100;
  const light = l / 100;
  const k = (n: number) => (n + h / 30) % 12;
  const a = sat * Math.min(light, 1 - light);
  const f = (n: number) =>
    light - a * Math.max(-1, Math.min(k(n) - 3, Math.min(9 - k(n), 1)));
  return [f(0) * 255, f(8) * 255, f(4) * 255];
}

export function parseColor(input: string): Rgb {
  const color = input.trim();
  const hsl = /^hsla?\(\s*([\d.]+)\s*,\s*([\d.]+)%\s*,\s*([\d.]+)%/i.exec(
    color,
  );
  if (hsl) {
    return hslToRgb(Number(hsl[1]), Number(hsl[2]), Number(hsl[3]));
  }
  let hex = color.replace('#', '');
  if (hex.length === 3) {
    hex = hex
      .split('')
      .map((c) => c + c)
      .join('');
  }
  if (!/^[0-9a-f]{6}$/i.test(hex)) {
    throw new Error(`Unsupported color: ${input}`);
  }
  return [
    parseInt(hex.slice(0, 2), 16),
    parseInt(hex.slice(2, 4), 16),
    parseInt(hex.slice(4, 6), 16),
  ];
}

export function toHex([r, g, b]: Rgb): string {
  const part = (c: number) =>
    Math.round(Math.max(0, Math.min(255, c)))
      .toString(16)
      .padStart(2, '0');
  return `#${part(r)}${part(g)}${part(b)}`;
}

/** Mixes `amount` (0..1) of `fg` into `bg`, like CSS `color-mix(in srgb)`. */
export function mix(fg: string, bg: string, amount: number): string {
  const a = parseColor(fg);
  const b = parseColor(bg);
  return toHex([
    a[0] * amount + b[0] * (1 - amount),
    a[1] * amount + b[1] * (1 - amount),
    a[2] * amount + b[2] * (1 - amount),
  ]);
}

export function relativeLuminance(color: string): number {
  const toLinear = (c: number) => {
    const v = c / 255;
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
  };
  const [r, g, b] = parseColor(color);
  return 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b);
}

export function contrastRatio(fg: string, bg: string): number {
  const l1 = relativeLuminance(fg);
  const l2 = relativeLuminance(bg);
  return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
}
