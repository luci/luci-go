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
