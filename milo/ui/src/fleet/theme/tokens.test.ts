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

import {
  borders,
  controlHeights,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
  statusTones,
} from './tokens';

describe('Fleet design tokens', () => {
  it('defines accessible border and divider contrasts', () => {
    // #dadce0: GM3 outlineVariant boundary for cards/tables against white
    expect(fleetColors.border).toBe('#dadce0');
    // #e0e2ec: High-contrast row divider
    expect(fleetColors.divider).toBe('#e0e2ec');
    // #c4c7c5: Emphasized outline for table header bottom border
    expect(fleetColors.borderEmphasis).toBe('#c4c7c5');
    // #f3f6fc: GM3 tonal fill for hover and headers
    expect(fleetColors.surfaceTonal).toBe('#f3f6fc');
    // #ffffff: Pure surface
    expect(fleetColors.surface).toBe('#ffffff');
    // #f8fafd: Subtle canvas background
    expect(fleetColors.canvas).toBe('#f8fafd');
  });

  it('exports composite border rules matching semantic tokens', () => {
    expect(borders.default).toBe('1px solid #dadce0');
    expect(borders.divider).toBe('1px solid #e0e2ec');
    expect(borders.emphasis).toBe('1px solid #c4c7c5');
  });

  it('defines GM3 corner radius scale', () => {
    expect(radii.sm).toBe('6px');
    expect(radii.md).toBe('12px');
    expect(radii.pill).toBe('9999px');
  });

  it('defines standard control heights and icon sizes', () => {
    expect(controlHeights.button).toBe(32);
    expect(controlHeights.chip).toBe(24);
    expect(controlHeights.input).toBe(40);
    expect(iconSizes.control).toBe(16);
    expect(iconSizes.logo).toBe(20);
    expect(fontSizes.label).toBe(13);
    expect(fontSizes.body).toBe(14);
  });

  it('defines display, text and monospace font families', () => {
    expect(fontFamilies.display).toContain('Google Sans');
    expect(fontFamilies.text).toContain('Google Sans Text');
    expect(fontFamilies.mono).toContain('Roboto Mono');
  });

  it('defines accessible status tones with foreground, background and indicator dot', () => {
    const tones = [
      'success',
      'warning',
      'error',
      'info',
      'neutral',
      'special',
    ] as const;
    for (const tone of tones) {
      expect(statusTones[tone]).toBeDefined();
      expect(statusTones[tone].fg).toBeTruthy();
      expect(statusTones[tone].bg).toBeTruthy();
      expect(statusTones[tone].dot).toBeTruthy();
    }
  });

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
