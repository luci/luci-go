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

import { axisLeft, select } from 'd3';
import { useEffect, useMemo, useRef } from 'react';

import {
  ParsedTestVariantBranchName,
  TestVariantBranchDef,
} from '@/analysis/types';
import { getLongestCommonPrefix } from '@/generic_libs/tools/string_utils';

import { LabelBox } from './common';
import { SIDE_PANEL_WIDTH } from './constants';
import { useConfig } from './context';

export interface SidePanelProps {
  readonly testVariantBranches: readonly TestVariantBranchDef[];
}

export function SidePanel({ testVariantBranches }: SidePanelProps) {
  const { criticalVariantKeys, yScale, testVariantBranchCount, rowHeight } =
    useConfig();

  const gridLineElement = useRef<SVGGElement | null>(null);
  useEffect(() => {
    const gridLines = axisLeft(yScale)
      .ticks(testVariantBranchCount)
      .tickSize(-SIDE_PANEL_WIDTH)
      .tickFormat(() => '');
    gridLines(select(gridLineElement.current!));
  }, [yScale, testVariantBranchCount]);
  const height = yScale.range()[1];

  const commonPrefix = useMemo(
    () => getLongestCommonPrefix(testVariantBranches.map((tvb) => tvb.testId)),
    [testVariantBranches],
  );

  return (
    <svg
      css={{
        gridArea: 'side-panel',
        position: 'sticky',
        left: 'var(--accumulated-left)',
        zIndex: 2,
        background: 'white',
      }}
      height={height}
      width={SIDE_PANEL_WIDTH}
    >
      <g
        ref={gridLineElement}
        css={{
          '& line,path': { stroke: 'var(--divider-color)' },
        }}
      />
      <path
        d={`m0.5,-1v${height + 2}m${SIDE_PANEL_WIDTH - 1},0v${-height - 2}`}
        stroke="var(--divider-color)"
      />
      {testVariantBranches.map((tvb, i) => (
        <g
          key={ParsedTestVariantBranchName.toString(tvb)}
          transform={`translate(0, ${yScale(i)})`}
        >
          <foreignObject height={rowHeight} width={SIDE_PANEL_WIDTH}>
            <LabelBox title={tvb.testId}>
              {commonPrefix && '...'}
              {tvb.testId.slice(commonPrefix.length)}
            </LabelBox>
            {criticalVariantKeys.map((k) => (
              <LabelBox key={k}>{tvb.variant?.def[k]}</LabelBox>
            ))}
          </foreignObject>
        </g>
      ))}
    </svg>
  );
}
