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

import { createTheme } from '@mui/material/styles';

import {
  BASE_WHITE_TOOLTIP_PAPER_SX,
  COMPACT_TOOLTIP_PAPER_SX,
  COMPACT_TOOLTIP_SLOT_PROPS,
  INFO_TOOLTIP_PAPER_SX,
  INFO_TOOLTIP_SLOT_PROPS,
  TRENDS_TOOLTIP_PAPER_SX,
  WHITE_TOOLTIP_ARROW_SX,
} from './info_tooltip_styles';

describe('info_tooltip_styles', () => {
  const theme = createTheme();

  it('defines white arrow styles with divider border function', () => {
    expect(WHITE_TOOLTIP_ARROW_SX.color).toBe('background.paper');
    const beforeSx = WHITE_TOOLTIP_ARROW_SX['&::before'] as {
      border: (t: typeof theme) => string;
    };
    expect(beforeSx.border(theme)).toBe(`1px solid ${theme.palette.divider}`);
  });

  it('defines base white tooltip paper sx matching hovercard visual language', () => {
    expect(BASE_WHITE_TOOLTIP_PAPER_SX.bgcolor).toBe('background.paper');
    expect(BASE_WHITE_TOOLTIP_PAPER_SX.color).toBe('text.primary');
    expect(BASE_WHITE_TOOLTIP_PAPER_SX.borderRadius).toBe(1.5);
    expect(BASE_WHITE_TOOLTIP_PAPER_SX.boxShadow).toBe(3);
    const borderFn = BASE_WHITE_TOOLTIP_PAPER_SX.border as (
      t: typeof theme,
    ) => string;
    expect(borderFn(theme)).toBe(`1px solid ${theme.palette.divider}`);
  });

  it('defines standard info tooltip paper sx with p: 2 and maxWidth: 450', () => {
    expect(INFO_TOOLTIP_PAPER_SX.p).toBe(2);
    expect(INFO_TOOLTIP_PAPER_SX.maxWidth).toBe(450);
    expect(INFO_TOOLTIP_PAPER_SX.pointerEvents).toBe('auto');
    expect(INFO_TOOLTIP_SLOT_PROPS.tooltip.sx).toBe(INFO_TOOLTIP_PAPER_SX);
  });

  it('defines compact tooltip paper sx with p: 1 and maxWidth: 320', () => {
    expect(COMPACT_TOOLTIP_PAPER_SX.p).toBe(1);
    expect(COMPACT_TOOLTIP_PAPER_SX.maxWidth).toBe(320);
    expect(COMPACT_TOOLTIP_SLOT_PROPS.tooltip.sx).toBe(
      COMPACT_TOOLTIP_PAPER_SX,
    );
  });

  it('defines trends chart tooltip paper sx with p: 1 and borderRadius: 1.5', () => {
    expect(TRENDS_TOOLTIP_PAPER_SX.bgcolor).toBe('background.paper');
    expect(TRENDS_TOOLTIP_PAPER_SX.p).toBe(1);
    expect(TRENDS_TOOLTIP_PAPER_SX.borderRadius).toBe(1.5);
    const borderFn = TRENDS_TOOLTIP_PAPER_SX.border as (
      t: typeof theme,
    ) => string;
    expect(borderFn(theme)).toBe(`1px solid ${theme.palette.divider}`);
  });
});
