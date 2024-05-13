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
import { axisTop, select } from 'd3';
import { useEffect, useRef } from 'react';

import { useTestVariantBranchesClient } from '@/analysis/hooks/prpc_clients';
import { QuerySourcePositionsRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { CELL_WIDTH } from './constants';
import { useConfig } from './context';

export function TopAxis() {
  const { criticalCommits, rowHeight, xScale } = useConfig();
  const client = useTestVariantBranchesClient();
  client.QuerySourcePositions(QuerySourcePositionsRequest.fromPartial({}));

  const gridLineElement = useRef<SVGGElement | null>(null);
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
      }}
      height={rowHeight}
      width={width}
    >
      <g
        ref={gridLineElement}
        css={{ '& line,path': { stroke: 'var(--divider-color)' } }}
      />
      <path
        d={`m0,${rowHeight - 0.5}h${width - 1}`}
        stroke="var(--divider-color)"
      />
      {criticalCommits.map((c, i) => (
        <g key={c} transform={`translate(${xScale(i)}, 0)`}>
          <foreignObject width={CELL_WIDTH} height={rowHeight}>
            <Box>{c}</Box>
          </foreignObject>
        </g>
      ))}
    </svg>
  );
}
