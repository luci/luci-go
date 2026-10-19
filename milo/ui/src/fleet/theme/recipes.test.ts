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
  actionRowSx,
  dockedTabStripSx,
  focusRingSx,
  iconButtonSx,
  metricTileSx,
  pageHeaderSx,
  pillInputSx,
  subMetricRowSx,
  surfaceCardClippedSx,
  surfaceCardSx,
  tabPanelSx,
  tonalPanelSx,
} from './recipes';
import {
  borders,
  controlHeights,
  elevations,
  fleetColors,
  radii,
} from './tokens';

describe('Fleet styling recipes', () => {
  it('defines surfaceCardSx matching token standards', () => {
    expect(surfaceCardSx).toBeDefined();
    expect(surfaceCardSx.backgroundColor).toBe(fleetColors.surface);
    expect(surfaceCardSx.border).toBe(borders.default);
    expect(surfaceCardSx.borderRadius).toBe(radii.md);
    expect(surfaceCardSx.boxShadow).toBe(elevations.card);
  });

  it('defines surfaceCardClippedSx with hidden overflow', () => {
    expect(surfaceCardClippedSx).toBeDefined();
    expect(surfaceCardClippedSx.overflow).toBe('hidden');
    expect(surfaceCardClippedSx.backgroundColor).toBe(fleetColors.surface);
  });

  it('defines tonalPanelSx without shadow', () => {
    expect(tonalPanelSx).toBeDefined();
    expect(tonalPanelSx.backgroundColor).toBe(fleetColors.surfaceTonal);
    expect(tonalPanelSx.border).toBe(borders.divider);
    expect(tonalPanelSx.borderRadius).toBe(radii.md);
  });

  it('defines pageHeaderSx and dockedTabStripSx with solid surface backgrounds', () => {
    expect(pageHeaderSx.backgroundColor).toBe(fleetColors.surface);
    expect(dockedTabStripSx.backgroundColor).toBe(fleetColors.surface);
    expect(dockedTabStripSx.borderBottom).toBe(borders.default);
  });

  it('defines tabPanelSx and actionRowSx layout properties', () => {
    expect(tabPanelSx.px).toBe(3);
    expect(tabPanelSx.py).toBe(2.5);
    expect(actionRowSx.display).toBe('flex');
    expect(actionRowSx.alignItems).toBe('center');
  });

  it('defines pillInputSx and iconButtonSx with control heights', () => {
    expect(pillInputSx.minHeight).toBe(controlHeights.input);
    expect(pillInputSx.borderRadius).toBe(`${controlHeights.input / 2}px`);
    expect(pillInputSx['&.MuiOutlinedInput-root fieldset'].borderRadius).toBe(
      `${controlHeights.input / 2}px`,
    );
    expect(iconButtonSx.width).toBe(controlHeights.button);
    expect(iconButtonSx.height).toBe(controlHeights.button);
    expect(iconButtonSx.borderRadius).toBe(radii.pill);
    expect(iconButtonSx['&:hover']).toEqual({
      backgroundColor: fleetColors.primaryContainer,
    });
  });

  it('defines metricTileSx, subMetricRowSx, and focusRingSx with 6px radius, tonal hover, and accessible focus rings', () => {
    expect(metricTileSx.borderRadius).toBe(radii.sm);
    expect(metricTileSx['&:hover'].backgroundColor).toBe(
      fleetColors.surfaceTonal,
    );
    expect(
      metricTileSx['&:focus-visible, &.Mui-focusVisible'].outline,
    ).toContain(fleetColors.primary);
    expect(
      metricTileSx['&:focus-visible, &.Mui-focusVisible'].outlineOffset,
    ).toBe(2);

    expect(subMetricRowSx.borderRadius).toBe(radii.sm);
    expect(subMetricRowSx['&:hover'].backgroundColor).toBe(
      fleetColors.surfaceTonal,
    );
    expect(
      subMetricRowSx['&:focus-visible, &.Mui-focusVisible'].outline,
    ).toContain(fleetColors.primary);
    expect(
      subMetricRowSx['&:focus-visible, &.Mui-focusVisible'].outlineOffset,
    ).toBe(2);

    expect(
      focusRingSx['&:focus-visible, &.Mui-focusVisible'].outline,
    ).toContain(fleetColors.primary);
    expect(
      focusRingSx['&:focus-visible, &.Mui-focusVisible'].outlineOffset,
    ).toBe(2);
  });
});
