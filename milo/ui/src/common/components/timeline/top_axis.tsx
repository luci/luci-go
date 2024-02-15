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

import { useTheme } from '@mui/material';
import { axisTop, select } from 'd3';
import { Duration } from 'luxon';
import { useEffect, useRef } from 'react';

import { displayDuration } from '@/common/tools/time_utils';

import { TEXT_HEIGHT, TEXT_MARGIN, TOP_AXIS_HEIGHT } from './constants';
import { useRulerState, useTimelineConfig } from './context';

function TopAxisRuler() {
  const state = useRulerState();
  const config = useTimelineConfig();
  const startMs = config.startTimeMs;
  const selectedMs = state ? config.xScale.invert(state).getTime() : null;

  return (
    <>
      <line
        opacity={state === null ? 0 : 1}
        x1={state || 0}
        x2={state || 0}
        y2={-TOP_AXIS_HEIGHT}
        stroke="red"
        pointerEvents="none"
      />
      <text
        opacity={state === null ? 0 : 1}
        x={(state || 0) + TEXT_MARGIN}
        y={-TEXT_HEIGHT - TEXT_MARGIN}
        fill="red"
        textAnchor="start"
      >
        {selectedMs ? (
          selectedMs > startMs ? (
            <>
              {displayDuration(Duration.fromMillis(selectedMs - startMs))} since
              start
            </>
          ) : (
            <>
              {displayDuration(Duration.fromMillis(startMs - selectedMs))}{' '}
              before start
            </>
          )
        ) : (
          ''
        )}
      </text>
    </>
  );
}

export function TopAxis() {
  const theme = useTheme();
  const config = useTimelineConfig();

  const topAxisElement = useRef<SVGGElement>(null);
  useEffect(() => {
    const topAxis = axisTop(config.xScale).ticks(config.timeInterval);
    topAxis(select(topAxisElement.current!));
  }, [config.xScale, config.timeInterval]);

  return (
    <svg
      width={config.bodyWidth}
      height={TOP_AXIS_HEIGHT}
      css={{
        gridArea: 'top-axis',
        position: 'sticky',
        top: 0,
        zIndex: 1,
        background: theme.palette.background.default,
      }}
    >
      <g transform="translate(0.5, 0.5)">
        <g transform={`translate(0, ${TOP_AXIS_HEIGHT - 1})`}>
          <g ref={topAxisElement}></g>
          <TopAxisRuler />
        </g>
      </g>
      <path
        d={`m${config.bodyWidth - 0.5},0v${TOP_AXIS_HEIGHT}`}
        stroke="currentcolor"
      />
    </svg>
  );
}
