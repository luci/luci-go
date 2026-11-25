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

import { getSeriesColor } from '../pages/chromeos/health/trends_chart_data';

import { contrastRatio } from './color_math';
import {
  darkPalette,
  FLEET_PALETTES,
  fleetThemeCssVars,
  lightPalette,
} from './palettes';
import { createFleetTheme } from './theme';
import { avatarColors, fleetColors, statusTones, toneColors } from './tokens';

describe('Fleet design tokens', () => {
  it('falls back to the light palette when no theme vars are set', () => {
    expect(fleetColors.canvas).toBe('var(--fc-canvas, #f8fafd)');
    expect(statusTones.success.fg).toBe('var(--fc-status-success-fg, #137333)');
    expect(toneColors.success.container).toBe(
      'var(--fc-tone-success-container, hsl(137, 40%, 86%))',
    );
    expect(avatarColors.onFill).toBe('var(--fc-avatar-on-fill, #ffffff)');
    expect(avatarColors.fills).toHaveLength(lightPalette.avatar.fills.length);
  });

  it('keeps user avatar fills and white monogram foreground identical across light and dark', () => {
    expect(darkPalette.avatar).toEqual(lightPalette.avatar);
    expect(lightPalette.avatar.onFill).toBe('#ffffff');
  });

  it('gives every token a value in both light and dark', () => {
    expect(Object.keys(fleetThemeCssVars(darkPalette)).sort()).toEqual(
      Object.keys(fleetThemeCssVars(lightPalette)).sort(),
    );
  });

  it.each(Object.values(FLEET_PALETTES).map((p) => [p.id, p] as const))(
    'enforces WCAG 2.1 AA contrast (>= 4.5:1) for text and status tones in %s',
    (_id, palette) => {
      const c = palette.colors;
      const pairs: [string, string, string][] = [];
      for (const [name, tone] of Object.entries(palette.status)) {
        pairs.push([`status.${name}`, tone.fg, tone.bg]);
      }
      for (const bg of [
        'surface',
        'canvas',
        'surfaceTonal',
        'tableHeader',
        'pageHeader',
        'controlSurface',
        'panelSubtle',
        'panelMuted',
      ] as const) {
        for (const fg of [
          'textPrimary',
          'textSecondary',
          'textTertiary',
          'primary',
        ] as const) {
          pairs.push([`${fg} on ${bg}`, c[fg], c[bg]]);
        }
      }
      for (const fg of ['textPrimary', 'textSecondary', 'primary'] as const) {
        pairs.push([`${fg} on shell`, c[fg], c.shell]);
      }
      pairs.push(['onPrimary on primary', c.onPrimary, c.primary]);
      pairs.push([
        'onPrimaryContainer on primaryContainer',
        c.onPrimaryContainer,
        c.primaryContainer,
      ]);
      // Filter chips render `primary` text on `primaryContainer`.
      pairs.push([
        'primary on primaryContainer',
        c.primary,
        c.primaryContainer,
      ]);
      pairs.push([
        'onNavSelected on navSelected',
        c.onNavSelected,
        c.navSelected,
      ]);
      pairs.push([
        'onUnknownState on unknownState',
        c.onUnknownState,
        c.unknownState,
      ]);
      pairs.push([
        'caution onContainer on container',
        palette.tones.caution.onContainer,
        palette.tones.caution.container,
      ]);

      const failures = pairs
        .map(([name, fg, bg]) => ({ name, ratio: contrastRatio(fg, bg) }))
        .filter(({ ratio }) => ratio < 4.5)
        .map(({ name, ratio }) => `${name}: ${ratio.toFixed(2)}`);
      expect(failures).toEqual([]);
    },
  );

  it('keeps dark tone text readable on its container and the surface', () => {
    const c = darkPalette.colors;
    const failures: string[] = [];
    for (const [name, t] of Object.entries(darkPalette.tones)) {
      const checks: [string, string, string][] = [
        ['onContainer/container', t.onContainer, t.container],
        ['onTint/container', c.onTint, t.container],
        ['main/surface', t.main, c.surface],
        ['mainStrong/surface', t.mainStrong, c.surface],
      ];
      if (name === 'info') {
        checks.push(['infoAccent/infoBanner', c.infoAccent, c.infoBanner]);
      }
      for (const [label, fg, bg] of checks) {
        const ratio = contrastRatio(fg, bg);
        if (ratio < 4.5) failures.push(`${name} ${label}: ${ratio.toFixed(2)}`);
      }
    }
    expect(failures).toEqual([]);
  });

  it('keeps typography identical and icon action colors readable across light and dark themes', () => {
    const light = createFleetTheme(lightPalette);
    const dark = createFleetTheme(darkPalette);

    for (const variant of [
      'h1',
      'h2',
      'h3',
      'h4',
      'h5',
      'h6',
      'subtitle1',
      'subtitle2',
      'body1',
      'body2',
      'caption',
      'button',
    ] as const) {
      expect(dark.typography[variant]).toEqual(light.typography[variant]);
    }

    // Guards against light baseTheme action.active leaking dark-on-dark icons.
    expect(
      contrastRatio(dark.palette.action.active, darkPalette.colors.shell),
    ).toBeGreaterThanOrEqual(4.5);
    expect(
      contrastRatio(dark.palette.action.active, darkPalette.colors.surface),
    ).toBeGreaterThanOrEqual(4.5);

    // Saturated dark error.main must meet AA on every resting and tonal surface.
    for (const bg of [
      darkPalette.colors.surface,
      darkPalette.colors.canvas,
      darkPalette.colors.surfaceTonal,
    ]) {
      expect(contrastRatio(dark.palette.error.main, bg)).toBeGreaterThanOrEqual(
        4.5,
      );
    }

    // Keeps hover/tonal fills perceptibly distinct from resting surfaces.
    expect(
      contrastRatio(
        darkPalette.colors.surfaceTonal,
        darkPalette.colors.surface,
      ),
    ).toBeGreaterThanOrEqual(1.2);
    expect(
      contrastRatio(
        darkPalette.colors.surfaceTonal,
        darkPalette.colors.pageHeader,
      ),
    ).toBeGreaterThanOrEqual(1.12);
  });

  it('keeps procedural overflow chart series readable on light and dark surfaces', () => {
    for (let slot = 12; slot < 36; slot++) {
      const color = getSeriesColor(slot);
      expect(
        contrastRatio(color, lightPalette.colors.surface),
      ).toBeGreaterThanOrEqual(3.0);
      expect(
        contrastRatio(color, darkPalette.colors.surface),
      ).toBeGreaterThanOrEqual(3.0);
    }
  });
});
