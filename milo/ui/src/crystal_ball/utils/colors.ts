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

import { GOLDEN_RATIO_CONJUGATE } from '@/crystal_ball/constants';

/**
 * Matches hex colors: #rgb, #rgba, #rrggbb and #rrggbbaa.
 */
const HEX_COLOR_REGEX = /^#(?:[0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/i;

/**
 * Matches CSS named colors (e.g. "red", "rebeccapurple"). Letters only, so it
 * cannot contain CSS structural characters.
 */
const NAMED_COLOR_REGEX = /^[a-z]{1,32}$/i;

/**
 * Matches rgb()/rgba()/hsl()/hsla() colors whose arguments only contain
 * numbers, units, separators and whitespace (e.g. "rgb(255, 0, 0)",
 * "hsl(120deg 50% 50% / 0.5)").
 */
const FUNCTIONAL_COLOR_REGEX =
  /^(?:rgba?|hsla?)\((?:[\d.\s,/%+-]|deg|turn|rad)*\)$/i;

/**
 * Returns whether `color` is a CSS color literal that is safe to interpolate
 * into generated CSS (e.g. MUI `sx` props).
 *
 * Colors such as `PerfChartSeries.color` come from stored dashboard state and
 * are passed to MUI style props, which do not escape values, so only known
 * color formats are accepted.
 */
export function isSafeCssColor(color: unknown): color is string {
  if (typeof color !== 'string') return false;
  return (
    HEX_COLOR_REGEX.test(color) ||
    NAMED_COLOR_REGEX.test(color) ||
    FUNCTIONAL_COLOR_REGEX.test(color)
  );
}

/**
 * Returns `color` if it is a safe CSS color literal, otherwise `undefined`.
 */
export function getSafeCssColor(color: unknown): string | undefined {
  return isSafeCssColor(color) ? color : undefined;
}

/**
 * Generates a distinct HEX color for a given series index using the golden ratio.
 */
export function generateColor(index: number): string {
  return hslToHex(((index * GOLDEN_RATIO_CONJUGATE) % 1) * 360);
}

/**
 * Converts an HSL color value to a HEX string.
 *
 * @param h Hue [0, 360]
 * @param s Saturation [0, 100]
 * @param l Lightness [0, 100]
 * @returns HEX color string (e.g., "#ff0000")
 */
export function hslToHex(h: number, s: number = 70, l: number = 50): string {
  l /= 100;
  const a = (s * Math.min(l, 1 - l)) / 100;
  const f = (n: number) => {
    const k = (n + h / 30) % 12;
    const color = l - a * Math.max(Math.min(k - 3, 9 - k, 1), -1);
    return Math.round(255 * color)
      .toString(16)
      .padStart(2, '0');
  };
  return `#${f(0)}${f(8)}${f(4)}`;
}
