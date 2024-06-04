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
      }}
      width={SIDE_PANEL_WIDTH}
      height={rowHeight}
    >
      <path
        d={`m-0.5,-0.5m0,${rowHeight}h${SIDE_PANEL_WIDTH}m0,0v${-rowHeight}`}
        stroke="var(--divider-color)"
      />
      <foreignObject width={SIDE_PANEL_WIDTH} height={rowHeight}>
        <LabelBox>Test ID</LabelBox>
        {criticalVariantKeys.map((k) => (
          <LabelBox key={k}>variant:{k}</LabelBox>
        ))}
      </foreignObject>
    </svg>
  );
}
