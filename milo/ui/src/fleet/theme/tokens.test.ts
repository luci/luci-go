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

import { fleetColors, statusTones } from './tokens';

describe('Fleet design tokens', () => {
  it('enforces WCAG 2.1 AA contrast (>= 4.5:1) for all text and status tones', () => {
    const relativeLuminance = (hex: string): number => {
      const clean = hex.replace('#', '');
      const r = parseInt(clean.slice(0, 2), 16) / 255;
      const g = parseInt(clean.slice(2, 4), 16) / 255;
      const b = parseInt(clean.slice(4, 6), 16) / 255;
      const toLinear = (c: number) =>
        c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
      return 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b);
    };
    const contrastRatio = (fg: string, bg: string): number => {
      const l1 = relativeLuminance(fg);
      const l2 = relativeLuminance(bg);
      return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
    };

    for (const [, tone] of Object.entries(statusTones)) {
      expect(contrastRatio(tone.fg, tone.bg)).toBeGreaterThanOrEqual(4.5);
    }

    const surfaces = [
      fleetColors.surface,
      fleetColors.canvas,
      fleetColors.surfaceTonal,
    ];
    const texts = [
      fleetColors.textPrimary,
      fleetColors.textSecondary,
      fleetColors.textTertiary,
      fleetColors.primary,
    ];
    for (const bg of surfaces) {
      for (const fg of texts) {
        expect(contrastRatio(fg, bg)).toBeGreaterThanOrEqual(4.5);
      }
    }
  });
});
