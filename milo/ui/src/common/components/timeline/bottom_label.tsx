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

import { useTheme } from '@mui/material';

import { BOTTOM_AXIS_HEIGHT, TEXT_HEIGHT, TEXT_MARGIN } from './constants';
import { useTimelineConfig } from './context';

export interface BottomLabelProps {
  readonly label: string;
}

export function BottomLabel({ label }: BottomLabelProps) {
  const theme = useTheme();
  const config = useTimelineConfig();

  return (
    <svg
      width={config.sidePanelWidth}
      height={BOTTOM_AXIS_HEIGHT}
      css={{
        gridArea: 'bottom-label',
        position: 'sticky',
        bottom: 'var(--accumulated-bottom)',
        left: 'var(--accumulated-left)',
        backgroundColor: theme.palette.background.default,
        zIndex: 2,
      }}
    >
      <text x={TEXT_MARGIN} y={TEXT_HEIGHT + TEXT_MARGIN / 2} fontWeight={500}>
        {label}
      </text>
      <path
        d={`m-0.5,0.5h${config.sidePanelWidth}m0,0v${BOTTOM_AXIS_HEIGHT}`}
        stroke="currentcolor"
      />
    </svg>
  );
}
