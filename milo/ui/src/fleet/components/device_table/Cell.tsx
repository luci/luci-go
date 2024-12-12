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

export const Cell = ({ value, colDef }: GridRenderCellParams) => {
  const ref = useRef<HTMLSpanElement>(null);

  return (
    <Tooltip
      title={value}
      disableHoverListener={
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
