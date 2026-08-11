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

import { Box } from '@mui/material';
import { DateTime } from 'luxon';
import {
  useMemo,
  useContext,
  useCallback,
  useState,
  useRef,
  useEffect,
} from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';

import {
  getStageLabel,
  getStageResultStatus,
} from '@/chronicle/utils/check_utils';
import { getBaseNodeId } from '@/chronicle/utils/id/get_base_node_id';
import { parseTimestamp } from '@/chronicle/utils/time_utils';
import {
  Body,
  BottomAxis,
  SidePanel,
  Timeline,
  TopAxis,
  TopLabel,
} from '@/common/components/timeline';
import { useDeclareTabId } from '@/generic_libs/components/routed_tabs/context';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';

import { ChronicleContext } from '../context';
import { InspectorPanel } from '../inspector_panel/inspector_panel';

import { StageRow } from './stage_row';
import { StageTimelineBar } from './stage_timeline_bar';
import { ROW_HEIGHT, STAGE_COLUMN_WIDTH, TimelineItem } from './types';

function useContainerWidth() {
  const [width, setWidth] = useState(0);
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new ResizeObserver((entries) => {
      if (entries[0]) setWidth(entries[0].contentRect.width);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return [ref, width] as const;
}

function TimelineView() {
  useDeclareTabId('timeline');

  const { graph, valueDataMap, selectedNodeId, setSelectedNodeId } =
    useContext(ChronicleContext);

  const [containerRef, containerWidth] = useContainerWidth();

  const bodyWidth = useMemo(
    () =>
      containerWidth
        ? Math.max(200, containerWidth - STAGE_COLUMN_WIDTH - 32)
        : 1200,
    [containerWidth],
  );

  const { items, timelineStart, timelineEnd } = useMemo(() => {
    if (!graph)
      return {
        items: [],
        timelineStart: DateTime.now(),
        timelineEnd: DateTime.now(),
      };

    const stages = Object.values(graph.stages) as Stage[];
    let minMs = Infinity;
    let maxMs = -Infinity;

    const timelineItems = stages
      .map((sv): TimelineItem | undefined => {
        if (!sv || !sv.stateHistory) return undefined;

        const attemptingStateHistory = sv.stateHistory.find(
          (s) => s.state === StageState.STAGE_STATE_ATTEMPTING,
        );
        const finalStateHistory = sv.stateHistory.find(
          (s) => s.state === StageState.STAGE_STATE_FINAL,
        );

        if (!sv.identifier?.id) {
          return undefined;
        }

        const start = parseTimestamp(attemptingStateHistory?.version?.ts);
        const end = parseTimestamp(finalStateHistory?.version?.ts);

        if (!start || !end) {
          return undefined;
        }

        minMs = Math.min(minMs, start.toMillis());
        maxMs = Math.max(maxMs, end.toMillis());

        return {
          id: sv.identifier.id,
          label: getStageLabel(sv, valueDataMap),
          start,
          end,
          stage: sv,
          resultStatus: getStageResultStatus(sv, valueDataMap),
        };
      })
      .filter((item): item is TimelineItem => !!item)
      .sort((a, b) => a.start.toMillis() - b.start.toMillis());

    return {
      items: timelineItems,
      timelineStart: DateTime.fromMillis(minMs),
      timelineEnd: DateTime.fromMillis(maxMs),
    };
  }, [graph, valueDataMap]);

  const baseSelectedNodeId = useMemo(
    () => getBaseNodeId(selectedNodeId, { includePrefix: false }),
    [selectedNodeId],
  );

  const selectedItem = useMemo(
    () =>
      baseSelectedNodeId
        ? items.find((item) => item.id === baseSelectedNodeId)
        : undefined,
    [items, baseSelectedNodeId],
  );

  const onInspectorClose = useCallback(() => {
    setSelectedNodeId(undefined);
  }, [setSelectedNodeId]);

  return (
    <PanelGroup
      direction="horizontal"
      style={{
        overflowY: 'visible',
        overflowX: 'clip',
        width: '100%',
      }}
    >
      <Panel minSize={30} style={{ overflowY: 'visible', overflowX: 'clip' }}>
        <Box ref={containerRef} sx={{ p: 2 }}>
          <Timeline
            startTime={timelineStart}
            endTime={timelineEnd}
            itemCount={items.length}
            itemHeight={ROW_HEIGHT}
            sidePanelWidth={STAGE_COLUMN_WIDTH}
            bodyWidth={bodyWidth}
          >
            <TopLabel label="Stage" />
            <TopAxis />
            <SidePanel
              content={(index) => {
                const item = items[index];
                const isSelected = item.id === baseSelectedNodeId;
                return (
                  <StageRow
                    item={item}
                    isSelected={isSelected}
                    onClick={() =>
                      setSelectedNodeId(isSelected ? undefined : item.id)
                    }
                  />
                );
              }}
            />
            <Body
              content={(index, xScale) => {
                const item = items[index];
                const isSelected = item.id === baseSelectedNodeId;
                return (
                  <StageTimelineBar
                    item={item}
                    xScale={xScale}
                    isSelected={isSelected}
                    onClick={() =>
                      setSelectedNodeId(isSelected ? undefined : item.id)
                    }
                  />
                );
              }}
            />
            <BottomAxis />
          </Timeline>
        </Box>
      </Panel>
      {selectedNodeId && selectedItem && (
        <>
          <PanelResizeHandle>
            <Box
              sx={{
                width: '8px',
                height: '100%',
                cursor: 'col-resize',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                bgcolor: 'action.hover',
                '&:hover': { bgcolor: 'action.selected' },
              }}
            >
              <Box sx={{ width: '2px', height: '24px', bgcolor: 'divider' }} />
            </Box>
          </PanelResizeHandle>
          <Panel
            defaultSize={30}
            minSize={20}
            style={{ overflowY: 'visible', overflowX: 'clip', minWidth: 0 }}
          >
            <Box sx={{ position: 'sticky', top: 0, height: '100vh' }}>
              <InspectorPanel
                nodeId={selectedNodeId}
                viewData={selectedItem.stage}
                valueDataMap={valueDataMap}
                onClose={onInspectorClose}
              />
            </Box>
          </Panel>
        </>
      )}
    </PanelGroup>
  );
}

export { TimelineView as Component };
