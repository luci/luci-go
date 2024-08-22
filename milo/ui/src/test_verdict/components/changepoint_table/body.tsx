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

import { axisBottom, axisLeft, select } from 'd3';
import { useEffect, useRef } from 'react';

import {
  OutputTestVariantBranch,
  ParsedTestVariantBranchName,
} from '@/analysis/types';

import { useConfig } from './hooks';
import { Row } from './row';

export interface BodyProps {
  readonly testVariantBranches: readonly OutputTestVariantBranch[];
}

export function Body({ testVariantBranches }: BodyProps) {
  const { xScale, yScale, testVariantBranchCount, criticalCommits } =
    useConfig();

  const width = xScale.range()[1];
  const height = yScale.range()[1];

  const horizontalGridLineElement = useRef<SVGGElement>(null);
  useEffect(() => {
    const gridLines = axisLeft(yScale)
      .ticks(testVariantBranchCount)
      .tickSize(-width)
      .tickFormat(() => '');
    gridLines(select(horizontalGridLineElement.current!));
  }, [yScale, testVariantBranchCount, width]);

  const verticalGridLineElement = useRef<SVGGElement>(null);
  useEffect(() => {
    const gridLines = axisBottom(xScale)
      .ticks(criticalCommits.length)
      .tickSize(height)
      .tickFormat(() => '');
    gridLines(select(verticalGridLineElement.current!));
  }, [xScale, criticalCommits.length, height]);

  return (
    <svg
      css={{
        gridArea: 'body',
        border: 'solid 1px var(--divider-color)',
      }}
      width={width}
      height={height}
    >
      <g
        ref={horizontalGridLineElement}
        css={{ '& line,path': { stroke: 'var(--divider-color)' } }}
      />
      <g
        ref={verticalGridLineElement}
        css={{ '& line,path': { stroke: 'var(--divider-color)' } }}
      />
      {testVariantBranches.map((tvb, i) => (
        <g
          key={ParsedTestVariantBranchName.toString(tvb)}
          transform={`translate(0, ${yScale(i)})`}
        >
          <Row testVariantBranch={tvb} />
        </g>
      ))}
    </svg>
  );
}
