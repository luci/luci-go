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
import { ScaleTime } from 'd3';
import { DateTime } from 'luxon';

import { OutputStep } from '@/build/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import {
  BUILD_STATUS_BG_COLOR_MAP,
  BUILD_STATUS_COLOR_MAP,
} from '@/common/constants/build';
import { ContainedForeignObject } from '@/generic_libs/components/contained_foreign_object';

import {
  TEXT_FONT_SIZE,
  TIME_SPAN_HEIGHT,
  TIME_SPAN_MIN_WIDTH,
} from './constants';
import { StepTooltip } from './tooltip';

export interface TimeSpanProps {
  readonly buildStartTime: DateTime;
  readonly index: readonly number[];
  readonly selfName: string;
  readonly step: OutputStep;
  readonly xScale: ScaleTime<number, number, never>;
}

export function TimeSpan({
  buildStartTime,
  index,
  selfName,
  step,
  xScale,
}: TimeSpanProps) {
  const theme = useTheme();

  if (!step.startTime) {
    return <></>;
  }

  const startTime = DateTime.fromISO(step.startTime);
  const endTime = step.endTime ? DateTime.fromISO(step.endTime) : null;
  const start = xScale(startTime);
  const end = endTime ? xScale(endTime) : xScale.range()[1];

  return (
    <>
      <rect
        x={start - TIME_SPAN_MIN_WIDTH / 2}
        y={-TIME_SPAN_HEIGHT / 2}
        width={end - start + TIME_SPAN_MIN_WIDTH}
        height={TIME_SPAN_HEIGHT}
        fill={BUILD_STATUS_BG_COLOR_MAP[step.status]}
        stroke={BUILD_STATUS_COLOR_MAP[step.status]}
      />
      <ContainedForeignObject
        fixedSizeChildren
        x={start}
        y={-TEXT_FONT_SIZE / 2}
      >
        <HtmlTooltip
          disableInteractive
          title={<StepTooltip step={step} buildStartTime={buildStartTime} />}
        >
          <Box
            sx={{
              display: 'inline-block',
              whiteSpace: 'nowrap',
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
      </ContainedForeignObject>
    </>
  );
}
