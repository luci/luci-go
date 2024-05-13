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
import { select, axisBottom } from 'd3';
import { useEffect, useRef } from 'react';

import { BOTTOM_AXIS_HEIGHT } from './constants';
import { useTimelineConfig } from './context';

export function BottomAxis() {
  const theme = useTheme();
  const config = useTimelineConfig();

  const bottomAxisElement = useRef<SVGGElement>(null);
  useEffect(() => {
    const bottomAxis = axisBottom(config.xScale).ticks(config.timeInterval);
    bottomAxis(select(bottomAxisElement.current!));
  }, [config.xScale, config.timeInterval]);

  return (
    <svg
      width={config.bodyWidth}
      height={BOTTOM_AXIS_HEIGHT}
      css={{
        gridArea: 'bottom-axis',
        position: 'sticky',
        bottom: 'var(--accumulated-bottom)',
        zIndex: 1,
        background: theme.palette.background.default,
      }}
    >
      <g transform="translate(0.5, 0.5)" ref={bottomAxisElement} />
      <path
        d={`m${config.bodyWidth - 0.5},0v${BOTTOM_AXIS_HEIGHT}`}
        stroke="currentcolor"
      />
    </svg>
  );
}
