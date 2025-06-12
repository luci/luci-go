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

import { Tooltip } from '@mui/material';
import { GridRenderCellParams } from '@mui/x-data-grid';
import { useRef } from 'react';

/**
 * mui-data-grid cell with custom styles and functionality to truncate
 * overflowing data and to show a tooltip when the value is truncated.
 */
export const CellWithTooltip = ({
  value,
  colDef,
  tooltipTitle,
}: GridRenderCellParams & { tooltipTitle?: string }) => {
  const ref = useRef<HTMLSpanElement>(null);

  return (
    <Tooltip
      title={tooltipTitle ?? value}
      disableHoverListener={
        // Only show the hover tooltip when the column is smaller than the content.
        !!ref.current && ref.current?.offsetWidth + 20 <= (colDef.width ?? 0)
      }
    >
      <span
        ref={ref}
        style={{
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          maxWidth: '100%',
        }}
      >
        {value}
      </span>
    </Tooltip>
  );
};
