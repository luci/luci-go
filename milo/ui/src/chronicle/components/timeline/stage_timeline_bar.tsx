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

import { DateTime } from 'luxon';
import { useMemo } from 'react';

import { getStageColors } from '@/chronicle/utils/styles';

import { ProgressSegment } from './segments';
import {
  BAR_BORDER_RADIUS,
  BAR_HEIGHT,
  SEGMENT_LABEL_MIN_WIDTH_PX,
  SELECTED_BAR_STYLE,
  TimelineItem,
} from './types';

interface StageTimelineBarProps {
  item: TimelineItem;
  xScale: (date: DateTime) => number;
  isSelected: boolean;
  onClick: () => void;
}

function formatDuration(segment: ProgressSegment): string {
  return segment.end
    .diff(segment.start)
    .rescale()
    .toHuman({ unitDisplay: 'short', maximumFractionDigits: 0 });
}

export function StageTimelineBar({
  item,
  xScale,
  isSelected,
  onClick,
}: StageTimelineBarProps) {
  const colors = getStageColors(item.resultStatus);
  const fill = isSelected ? SELECTED_BAR_STYLE.fill : colors.bg;
  const stroke = isSelected ? SELECTED_BAR_STYLE.stroke : colors.border;

  const attemptGroups = useMemo(() => {
    if (!item.segments?.length) {
      return [];
    }
    const segmentsByAttempt = new Map<number, ProgressSegment[]>();
    for (const seg of item.segments) {
      const group = segmentsByAttempt.get(seg.attemptNumber);
      if (group) {
        group.push(seg);
      } else {
        segmentsByAttempt.set(seg.attemptNumber, [seg]);
      }
    }
    return Array.from(segmentsByAttempt.values());
  }, [item.segments]);

  // Render a single fallback bar across the stage's duration when no progress
  // segments are available (e.g. stages without attempt progress messages).
  if (attemptGroups.length === 0) {
    const xStart = xScale(item.start);
    const width = Math.max(2, xScale(item.end) - xStart);
    return (
      <rect
        x={xStart}
        y={-BAR_HEIGHT / 2}
        width={width}
        height={BAR_HEIGHT}
        fill={fill}
        stroke={stroke}
        strokeWidth={isSelected ? 2 : 1}
        rx={BAR_BORDER_RADIUS}
        style={{ cursor: 'pointer' }}
        onClick={onClick}
      />
    );
  }

  return (
    <g onClick={onClick} style={{ cursor: 'pointer' }}>
      {attemptGroups.map((group) => {
        const { attemptNumber } = group[0];
        const attemptX = xScale(group[0].start);
        const attemptW = Math.max(
          2,
          xScale(group[group.length - 1].end) - attemptX,
        );
        const clipId = `clip-stage-${item.id}-att-${attemptNumber}`;

        return (
          <g key={`attempt-${attemptNumber}`}>
            <clipPath id={clipId}>
              <rect
                x={attemptX}
                y={-BAR_HEIGHT / 2}
                width={attemptW}
                height={BAR_HEIGHT}
                rx={BAR_BORDER_RADIUS}
              />
            </clipPath>
            <g clipPath={`url(#${clipId})`}>
              {group.map((seg, idx) => {
                const segX = xScale(seg.start);
                const segW = Math.max(1, xScale(seg.end) - segX);
                const showLabel =
                  seg.label && segW >= SEGMENT_LABEL_MIN_WIDTH_PX;
                const tooltip = `Attempt ${seg.attemptNumber}: ${seg.rawMessage || seg.label} (${formatDuration(seg)})`;
                return (
                  <g
                    key={`${seg.attemptNumber}-${idx}-${seg.start.toMillis()}`}
                  >
                    <title>{tooltip}</title>
                    <rect
                      x={segX}
                      y={-BAR_HEIGHT / 2}
                      width={segW}
                      height={BAR_HEIGHT}
                      fill={fill}
                    />
                    {idx > 0 && (
                      <line
                        x1={segX}
                        y1={-BAR_HEIGHT / 2}
                        x2={segX}
                        y2={BAR_HEIGHT / 2}
                        stroke="rgba(0, 0, 0, 0.4)"
                        strokeWidth={1}
                      />
                    )}
                    {showLabel && (
                      <text
                        x={segX + 4}
                        y={0}
                        dominantBaseline="middle"
                        fill={colors.text}
                        fontSize={10}
                        style={{ pointerEvents: 'none', userSelect: 'none' }}
                      >
                        {seg.label}
                      </text>
                    )}
                  </g>
                );
              })}
            </g>
            <rect
              x={attemptX}
              y={-BAR_HEIGHT / 2}
              width={attemptW}
              height={BAR_HEIGHT}
              fill="none"
              stroke={stroke}
              strokeWidth={isSelected ? 2 : 1}
              rx={BAR_BORDER_RADIUS}
              style={{ pointerEvents: 'none' }}
            />
          </g>
        );
      })}
    </g>
  );
}
