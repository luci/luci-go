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

import { DateTime } from 'luxon';

import {
  BottomAxis,
  BottomLabel,
  TopAxis,
  TopLabel,
} from '@/common/components/timeline';
import { Body } from '@/common/components/timeline/body';
import { SidePanel } from '@/common/components/timeline/side_panel';
import { Timeline } from '@/common/components/timeline/timeline';
import { NUMERIC_TIME_FORMAT } from '@/common/tools/time_utils';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';

import { ITEM_HEIGHT, SIDE_PANEL_WIDTH } from './constants';
import { SidePanelItem } from './side_panel_item';
import { TimeSpan } from './time_span';

export interface IncludedInvocationsTimelineProps {
  readonly invocation: Invocation;
}

export function IncludedInvocationsTimeline({
  invocation,
}: IncludedInvocationsTimelineProps) {
  const createTime = DateTime.fromISO(invocation.createTime!);
  const finalizeTime = invocation.finalizeTime
    ? DateTime.fromISO(invocation.finalizeTime)
    : null;
  const endTime = finalizeTime || DateTime.now();
  return (
    <Timeline
      startTime={createTime}
      endTime={endTime}
      itemCount={invocation.includedInvocations.length}
      itemHeight={ITEM_HEIGHT}
      sidePanelWidth={SIDE_PANEL_WIDTH}
      bodyWidth={1440}
    >
      <TopLabel
        label={`Create Time: ${createTime.toFormat(NUMERIC_TIME_FORMAT)}`}
      />
      <TopAxis />
      <SidePanel
        content={(index) => (
          <SidePanelItem
            invName={invocation.includedInvocations[index]}
            parentCreateTime={createTime}
          />
        )}
      />
      <Body
        content={(index, xScale) => (
          <TimeSpan
            invName={invocation.includedInvocations[index]}
            xScale={xScale}
          />
        )}
      />
      <BottomLabel
        label={`Finalize time: ${
          finalizeTime ? finalizeTime.toFormat(NUMERIC_TIME_FORMAT) : 'N/A'
        }`}
      />
      <BottomAxis />
    </Timeline>
  );
}
