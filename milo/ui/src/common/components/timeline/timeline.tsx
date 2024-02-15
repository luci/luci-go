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

import { Box, styled } from '@mui/material';
import { scaleLinear, scaleTime, timeMillisecond } from 'd3';
import { DateTime } from 'luxon';
import { ReactNode, useMemo } from 'react';

import { PREDEFINED_TIME_INTERVALS } from '@/common/constants/time';
import { roundDown } from '@/generic_libs/tools/utils';

import { V_GRID_LINE_MAX_GAP } from './constants';
import { TimelineConfig, TimelineContextProvider } from './context';

const Container = styled(Box)`
  display: grid;
  grid-template-areas:
    'top-label top-axis'
    'side-panel body'
    'bottom-label bottom-axis';
  grid-template-columns: auto 1fr;
`;

export interface TimelineProps {
  readonly startTime: DateTime;
  readonly endTime: DateTime;
  readonly itemCount: number;
  readonly itemHeight: number;
  readonly sidePanelWidth: number;
  readonly bodyWidth: number;
  readonly children: ReactNode;
}

export function Timeline({
  startTime,
  endTime,
  itemCount,
  bodyWidth,
  sidePanelWidth,
  itemHeight,
  children,
}: TimelineProps) {
  const startTimeMs = startTime.toMillis();
  const endTimeMs = endTime.toMillis();

  const config = useMemo<TimelineConfig>(() => {
    const maxInterval =
      (endTimeMs - startTimeMs) / (bodyWidth / V_GRID_LINE_MAX_GAP);

    return {
      startTimeMs,
      itemCount,
      itemHeight,
      sidePanelWidth,
      bodyWidth,
      xScale: scaleTime()
        .domain([startTimeMs, endTimeMs])
        // Ensure the left border is not rendered.
        .range([-1, bodyWidth]),
      yScale: scaleLinear()
        .domain([0, itemCount])
        // Ensure the top border is not rendered.
        .range([-1, itemCount * itemHeight]),
      timeInterval: timeMillisecond.every(
        roundDown(maxInterval, PREDEFINED_TIME_INTERVALS),
      )!,
    };
  }, [
    startTimeMs,
    endTimeMs,
    itemCount,
    itemHeight,
    bodyWidth,
    sidePanelWidth,
  ]);

  return (
    <TimelineContextProvider config={config}>
      <Container>{children}</Container>
    </TimelineContextProvider>
  );
}
