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

import { Box } from '@mui/material';

import { LabelBox } from './common';
import { SIDE_PANEL_WIDTH } from './constants';
import { useConfig } from './context';

export function TopLabel() {
  const { criticalVariantKeys, rowHeight } = useConfig();

  return (
    <svg
      css={{
        gridArea: 'top-label',
        position: 'sticky',
        left: 'var(--accumulated-left)',
        top: 'var(--accumulated-top)',
        zIndex: 3,
        background: 'white',
        border: 'solid 1px var(--divider-color)',
      }}
      width={SIDE_PANEL_WIDTH}
      height={rowHeight}
    >
      <foreignObject
        width={SIDE_PANEL_WIDTH}
        height={rowHeight}
        css={{ position: 'relative' }}
      >
        <Box
          sx={{
            top: '50%',
            transform: 'translateY(-50%)',
            position: 'relative',
          }}
        >
          <LabelBox sx={{ fontWeight: 'bold' }}>Test ID</LabelBox>
          {criticalVariantKeys.map((k) => (
            <LabelBox key={k}>variant:{k}</LabelBox>
          ))}
        </Box>
        <Box sx={{ position: 'absolute', right: '5px', top: 0 }}>
          <LabelBox sx={{ fontWeight: 'bold' }}>Commits</LabelBox>
        </Box>
      </foreignObject>
      <path
        d={`M${SIDE_PANEL_WIDTH * (3 / 5)},0L${SIDE_PANEL_WIDTH * (4 / 5)},${rowHeight}`}
        stroke="var(--divider-color)"
      />
    </svg>
  );
}
