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

import {
  BAR_HEIGHT,
  BAR_STYLE,
  SELECTED_BAR_STYLE,
  STAGE_COLUMN_WIDTH,
  TimelineItem,
} from './types';

interface StageRowProps {
  item: TimelineItem;
  isSelected: boolean;
  onClick: () => void;
}

export function StageRow({ item, isSelected, onClick }: StageRowProps) {
  return (
    <g style={{ cursor: 'pointer' }} onClick={onClick}>
      <rect
        x={4}
        y={-BAR_HEIGHT / 2}
        width={STAGE_COLUMN_WIDTH - 8}
        height={BAR_HEIGHT}
        fill={isSelected ? SELECTED_BAR_STYLE.fill : BAR_STYLE.fill}
        stroke={isSelected ? SELECTED_BAR_STYLE.stroke : BAR_STYLE.stroke}
        strokeWidth={isSelected ? 2 : 1}
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
            fontWeight: isSelected ? 'bold' : 'normal',
            color: isSelected ? SELECTED_BAR_STYLE.color : 'inherit',
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
}
