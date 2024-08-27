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

import { Box, CircularProgress } from '@mui/material';
import { DateTime } from 'luxon';
import { useMemo } from 'react';

import { OutputBuild } from '@/build/types';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  Body,
  BottomAxis,
  BottomLabel,
  SidePanel,
  Timeline,
  TopAxis,
  TopLabel,
} from '@/common/components/timeline';
import { NUMERIC_TIME_FORMAT } from '@/common/tools/time_utils';
import { useTabId } from '@/generic_libs/components/routed_tabs';
import { CategoryTree } from '@/generic_libs/tools/category_tree';
import { NonNullableProps } from '@/generic_libs/types';

import { useBuild } from '../context';

import { ITEM_HEIGHT, SIDE_PANEL_WIDTH } from './constants';
import { SidePanelItem } from './side_panel_item';
import { TimeSpan } from './time_span';

export function TimelineTab() {
  const build = useBuild();
  if (!build) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (!build.startTime) {
    return <span>Build were not started.</span>;
  }

  if (!build.steps.length) {
    return <span>Build does not have any steps</span>;
  }

  return (
    <TimelineTabImpl
      build={build as NonNullableProps<OutputBuild, 'startTime'>}
    />
  );
}

interface TimelineTabImplProps {
  readonly build: NonNullableProps<OutputBuild, 'startTime'>;
}

function TimelineTabImpl({ build }: TimelineTabImplProps) {
  const steps = useMemo(() => {
    const tree = new CategoryTree(
      build.steps.map((step) => {
        const splitName = step.name.split('|');
        return [
          splitName,
          {
            selfName: splitName[splitName.length - 1],
            step,
          },
        ];
      }),
    );
    return [...tree.enumerate()].map(([index, step]) => ({
      index: index,
      selfName: step.selfName,
      step: step.step,
    }));
  }, [build]);

  const startTime = DateTime.fromISO(build.startTime);
  const endTime = build.endTime
    ? DateTime.fromISO(build.endTime)
    : DateTime.now();

  return (
    <Timeline
      startTime={startTime}
      endTime={endTime}
      itemCount={steps.length}
      itemHeight={ITEM_HEIGHT}
      sidePanelWidth={SIDE_PANEL_WIDTH}
      bodyWidth={1440}
    >
      <TopLabel
        label={`Start Time: ${startTime.toFormat(NUMERIC_TIME_FORMAT)}`}
      />
      <TopAxis />
      <SidePanel
        content={(i) => {
          const step = steps[i];
          return (
            <SidePanelItem
              buildStartTime={startTime}
              index={step.index}
              selfName={step.selfName}
              step={step.step}
            />
          );
        }}
      />
      <Body
        content={(i, xScale) => {
          const step = steps[i];
          return (
            <TimeSpan
              buildStartTime={startTime}
              index={step.index}
              selfName={step.selfName}
              step={step.step}
              xScale={xScale}
            />
          );
        }}
      />
      <BottomLabel
        label={`End Time: ${
          build.endTime ? endTime.toFormat(NUMERIC_TIME_FORMAT) : 'N/A'
        }`}
      />
      <BottomAxis />
    </Timeline>
  );
}

export function Component() {
  useTabId('timeline');

  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="timeline">
      <TimelineTab />
    </RecoverableErrorBoundary>
  );
}
