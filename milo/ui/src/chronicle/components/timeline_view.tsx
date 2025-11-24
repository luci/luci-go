// Copyright 2025 The LUCI Authors.
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

import { Box } from '@mui/material';
import { DateTime } from 'luxon';
import { useMemo, useContext } from 'react';

import {
  Body,
  BottomAxis,
  SidePanel,
  Timeline,
  TopAxis,
  TopLabel,
} from '@/common/components/timeline';
import { useDeclareTabId } from '@/generic_libs/components/routed_tabs/context';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';
import { StageView } from '@/proto/turboci/graph/orchestrator/v1/stage_view.pb';

import { ChronicleContext } from './chronicle_context';

const ROW_HEIGHT = 30;
const BAR_HEIGHT = 24;
const STAGE_COLUMN_WIDTH = 300;
const BAR_STYLE = {
  fill: 'var(--success-bg-color)',
  stroke: 'var(--success-color)',
};

interface TimelineItem {
  id: string;
  label: string;
  start: DateTime;
  end: DateTime;
  stage: StageView;
}

function TimelineView() {
  useDeclareTabId('timeline');

  const { graph } = useContext(ChronicleContext);

  const { items, timelineStart, timelineEnd } = useMemo(() => {
    if (!graph)
      return {
        items: [],
        timelineStart: DateTime.now(),
        timelineEnd: DateTime.now(),
      };

    const stages = Object.values(graph.stages);
    let minMs = Infinity;
    let maxMs = -Infinity;

    const timelineItems: TimelineItem[] = stages
      .map((sv) => {
        if (!sv.stage || !sv.stage.stateHistory) return undefined;

        // Calculate start and end times based on stage edits.
        // We take the edit moving state into ATTEMPTING as the start time
        // and the edit moving state to FINAL as the end time.
        const attemptingStateHistory = sv.stage.stateHistory.find(
          (s) => s.state === StageState.STAGE_STATE_ATTEMPTING,
        );
        const finalStateHistory = sv.stage.stateHistory.find(
          (s) => s.state === StageState.STAGE_STATE_FINAL,
        );

        if (
          !sv.stage?.identifier?.id ||
          !attemptingStateHistory?.version?.ts ||
          !finalStateHistory?.version?.ts
        ) {
          return undefined;
        }

        const start = DateTime.fromISO(attemptingStateHistory.version.ts);
        const end = DateTime.fromISO(finalStateHistory.version.ts);

        if (!start.isValid || !end.isValid) return undefined;

        minMs = Math.min(minMs, start.toMillis());
        maxMs = Math.max(maxMs, end.toMillis());

        return {
          id: sv.stage.identifier.id,
          label: sv.stage.identifier.id,
          start,
          end,
          stage: sv,
        };
      })
      .filter((item): item is TimelineItem => !!item)
      .sort((a, b) => a.start.toMillis() - b.start.toMillis());

    return {
      items: timelineItems,
      timelineStart: DateTime.fromMillis(minMs),
      timelineEnd: DateTime.fromMillis(maxMs),
    };
  }, [graph]);

  return (
    <Box sx={{ p: 2 }}>
      <Timeline
        startTime={timelineStart}
        endTime={timelineEnd}
        itemCount={items.length}
        itemHeight={ROW_HEIGHT}
        sidePanelWidth={STAGE_COLUMN_WIDTH}
        bodyWidth={1200}
      >
        <TopLabel label="Stage" />
        <TopAxis />
        <SidePanel
          content={(index) => {
            const item = items[index];
            return (
              <g>
                <rect
                  x={4}
                  y={-BAR_HEIGHT / 2}
                  width={STAGE_COLUMN_WIDTH - 8}
                  height={BAR_HEIGHT}
                  fill={BAR_STYLE.fill}
                  stroke={BAR_STYLE.stroke}
                  rx={2}
                />
                <foreignObject
                  x={4}
                  y={-BAR_HEIGHT / 2}
                  width={STAGE_COLUMN_WIDTH - 8}
                  height={BAR_HEIGHT}
                  style={{ pointerEvents: 'none' }}
                >
                  <Box
                    sx={{
                      height: '100%',
                      display: 'flex',
                      alignItems: 'center',
                      px: 1,
                      fontSize: '12px',
                      overflow: 'hidden',
                      whiteSpace: 'nowrap',
                      textOverflow: 'ellipsis',
                    }}
                  >
                    {item.label}
                  </Box>
                </foreignObject>
              </g>
            );
          }}
        />
        <Body
          content={(index, xScale) => {
            const item = items[index];
            const xStart = xScale(item.start);
            const xEnd = xScale(item.end);
            const width = Math.max(2, xEnd - xStart);

            return (
              <rect
                x={xStart}
                y={-BAR_HEIGHT / 2}
                width={width}
                height={BAR_HEIGHT}
                fill={BAR_STYLE.fill}
                stroke={BAR_STYLE.stroke}
                rx={2}
              />
            );
          }}
        />
        <BottomAxis />
      </Timeline>
    </Box>
  );
}

export { TimelineView as Component };
