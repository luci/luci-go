// Copyright 2024 The LUCI Authors.
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

import { Box, Link, useTheme } from '@mui/material';
import { DateTime } from 'luxon';

import { OutputStep } from '@/build/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import {
  BUILD_STATUS_BG_COLOR_MAP,
  BUILD_STATUS_COLOR_MAP,
} from '@/common/constants/build';

import {
  SIDE_PANEL_PADDING,
  SIDE_PANEL_RECT_PADDING,
  SIDE_PANEL_WIDTH,
  SUB_STEP_OFFSET,
  TEXT_FONT_SIZE,
  TIME_SPAN_HEIGHT,
} from './constants';
import { StepTooltip } from './tooltip';

export interface SidePanelItemProps {
  readonly index: readonly number[];
  readonly selfName: string;
  readonly step: OutputStep;
  readonly buildStartTime: DateTime;
}

export function SidePanelItem({
  index,
  selfName,
  step,
  buildStartTime,
}: SidePanelItemProps) {
  const theme = useTheme();

  return (
    <>
      <rect
        x={SIDE_PANEL_PADDING}
        y={-TIME_SPAN_HEIGHT / 2}
        width={SIDE_PANEL_WIDTH - 2 * SIDE_PANEL_PADDING}
        height={TIME_SPAN_HEIGHT}
        fill={BUILD_STATUS_BG_COLOR_MAP[step.status]}
        stroke={BUILD_STATUS_COLOR_MAP[step.status]}
      />
      <foreignObject
        x={SIDE_PANEL_PADDING}
        y={-TEXT_FONT_SIZE / 2}
        width={SIDE_PANEL_WIDTH - 2 * SIDE_PANEL_PADDING}
        height={TEXT_FONT_SIZE + 2}
      >
        <HtmlTooltip
          arrow
          disableInteractive
          title={<StepTooltip step={step} buildStartTime={buildStartTime} />}
        >
          <Box
            sx={{
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              paddingRight: `${SIDE_PANEL_RECT_PADDING}px`,
              paddingLeft: `${
                SIDE_PANEL_RECT_PADDING + SUB_STEP_OFFSET * (index.length - 1)
              }px`,
            }}
          >
            {index.map((i) => i + 1).join('.')}.{' '}
            <Link
              href={step.logs[0]?.viewUrl}
              underline={step.logs.length ? 'always' : 'none'}
              target="_blank"
              rel="noopener"
              sx={{
                color: theme.palette.text.primary,
                textDecorationColor: theme.palette.text.primary,
              }}
            >
              {selfName}
            </Link>
          </Box>
        </HtmlTooltip>
      </foreignObject>
    </>
  );
}
