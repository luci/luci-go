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

import { axisTop, select } from 'd3';
import { useEffect, useRef } from 'react';

import { LabelBox } from './common';
import { CELL_WIDTH } from './constants';
import { useConfig } from './hooks';

export function TopAxis() {
  const { criticalCommits, rowHeight, xScale } = useConfig();

  const gridLineElement = useRef<SVGGElement>(null);
  useEffect(() => {
    const gridLines = axisTop(xScale)
      .ticks(criticalCommits.length)
      .tickSize(-rowHeight)
      .tickFormat(() => '');
    gridLines(select(gridLineElement.current!));
  }, [xScale, criticalCommits.length, rowHeight]);

  const width = xScale.range()[1];

  return (
    <svg
      css={{
        gridArea: 'top-axis',
        position: 'sticky',
        top: 'var(--accumulated-top)',
        zIndex: 2,
        backgroundColor: 'white',
        border: 'solid 1px var(--divider-color)',
      }}
      height={rowHeight}
      width={width}
    >
      <g
        ref={gridLineElement}
        css={{ '& line,path': { stroke: 'var(--divider-color)' } }}
        transform="translate(0, -1)"
      />
      {criticalCommits.map((c, i) => (
        <g key={c} transform={`translate(${xScale(i)}, 0)`}>
          <foreignObject width={CELL_WIDTH} height={rowHeight}>
            <LabelBox sx={{ textAlign: 'center', fontWeight: 'bold' }}>
              {c}
            </LabelBox>
          </foreignObject>
        </g>
      ))}
    </svg>
  );
}
