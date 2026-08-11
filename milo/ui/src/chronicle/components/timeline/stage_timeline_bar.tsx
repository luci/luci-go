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

import { getStageColors } from '@/chronicle/utils/styles';

import { BAR_HEIGHT, SELECTED_BAR_STYLE, TimelineItem } from './types';

interface StageTimelineBarProps {
  item: TimelineItem;
  xScale: (date: DateTime) => number;
  isSelected: boolean;
  onClick: () => void;
}

export function StageTimelineBar({
  item,
  xScale,
  isSelected,
  onClick,
}: StageTimelineBarProps) {
  const xStart = xScale(item.start);
  const xEnd = xScale(item.end);
  const width = Math.max(2, xEnd - xStart);
  const colors = getStageColors(item.resultStatus);
  const fill = isSelected ? SELECTED_BAR_STYLE.fill : colors.bg;
  const stroke = isSelected ? SELECTED_BAR_STYLE.stroke : colors.border;

  return (
    <rect
      x={xStart}
      y={-BAR_HEIGHT / 2}
      width={width}
      height={BAR_HEIGHT}
      fill={fill}
      stroke={stroke}
      strokeWidth={isSelected ? 2 : 1}
      rx={2}
      style={{ cursor: 'pointer' }}
      onClick={onClick}
    />
  );
}
