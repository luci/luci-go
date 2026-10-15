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
  surfaceCardClippedSx,
  surfaceCardSx,
  tonalPanelSx,
} from '@/fleet/theme/recipes';
import { borders, fleetColors } from '@/fleet/theme/tokens';

import {
  FIRST_COL_PADDING,
  fleetTableBodyCellSx,
  fleetTableHeaderSx,
  LAST_COL_PADDING,
  SELECT_COL_PADDING,
} from './fleet_column_header.styles';

describe('Table and Surface Styling Standards', () => {
  it('enforces high-contrast boundaries on surface cards and tables', () => {
    // Card container outer boundary must use #dadce0 (GM3 outlineVariant)
    expect(surfaceCardSx.border).toBe('1px solid #dadce0');
    expect(surfaceCardClippedSx.border).toBe('1px solid #dadce0');
    expect(surfaceCardSx.backgroundColor).toBe(fleetColors.surface);
  });

  it('enforces visible row dividers on tonal nested panels and list rows', () => {
    // Row dividers must use #e0e2ec, not washed out #eef2f8
    expect(tonalPanelSx.border).toBe('1px solid #e0e2ec');
    expect(borders.divider).toBe('1px solid #e0e2ec');
  });

  it('enforces emphasized bottom border and static padding rules on table column headers', () => {
    // Table header bottom border must separate header from data rows via #c4c7c5
    const headerInner = (
      fleetTableHeaderSx as Record<string, Record<string, unknown>>
    )['&&'];
    expect(headerInner).toBeDefined();
    expect(headerInner.borderBottom).toBe('1px solid #c4c7c5');
    expect(headerInner.backgroundColor).toBe('var(--header-bg-color)');
    expect(headerInner.userSelect).toBe('none');
    expect(headerInner.paddingLeft).toBe('var(--cell-padding-horizontal)');
    expect(headerInner['&.select-col']).toEqual({
      paddingLeft: SELECT_COL_PADDING,
      paddingRight: SELECT_COL_PADDING,
    });
    expect(headerInner['&:first-of-type:not(.select-col)']).toEqual({
      paddingLeft: FIRST_COL_PADDING,
    });
    expect(headerInner['&:last-of-type']).toEqual({
      paddingRight: LAST_COL_PADDING,
    });
  });

  it('defines static body cell styles driven by CSS variables and modifier classes', () => {
    const bodyCell = fleetTableBodyCellSx as Record<string, unknown>;
    expect(bodyCell.paddingTop).toBe('var(--cell-padding-vertical)');
    expect(bodyCell.paddingLeft).toBe('var(--cell-padding-horizontal)');
    expect(bodyCell['&.select-col']).toEqual({
      paddingLeft: SELECT_COL_PADDING,
      paddingRight: SELECT_COL_PADDING,
    });
    expect(bodyCell['&:first-of-type:not(.select-col)']).toEqual({
      paddingLeft: FIRST_COL_PADDING,
    });
    expect(bodyCell['&:last-of-type']).toEqual({
      paddingRight: LAST_COL_PADDING,
    });
    expect(bodyCell['&&.column-highlight']).toEqual({
      backgroundColor: fleetColors.primaryContainer,
      color: fleetColors.primary,
    });
  });
});
