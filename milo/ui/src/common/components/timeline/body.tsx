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

import { Box, styled } from '@mui/material';
import { ScaleTime, axisLeft, axisTop, select } from 'd3';
import { ReactNode, forwardRef, useEffect, useRef } from 'react';
import { Virtuoso } from 'react-virtuoso';

import {
  useRulerState,
  useRulerStateSetters,
  useTimelineConfig,
} from './context';

const Container = styled(Box)`
  grid-area: body;
`;

interface OptionalChildrenProps {
  readonly children?: ReactNode;
}

function BodyItem(props: OptionalChildrenProps) {
  return <g {...props} />;
}

function BodyRuler() {
  const state = useRulerState();
  const config = useTimelineConfig();

  return (
    <line
      opacity={state === null ? 0 : 1}
      stroke="red"
      pointerEvents="none"
      x1={state || 0}
      x2={state || 0}
      y2={config.itemCount * config.itemHeight}
    />
  );
}

const BodySvg = forwardRef<HTMLDivElement, OptionalChildrenProps>(
  function BodySvg({ children, ...props }, _ref) {
    const config = useTimelineConfig();
    const rulerStateSetters = useRulerStateSetters();
    const svgRef = useRef<SVGSVGElement | null>(null);

    const horizontalGridLineElement = useRef<SVGGElement | null>(null);
    useEffect(() => {
      const horizontalGridLines = axisLeft(config.yScale)
        .ticks(config.itemCount)
        .tickFormat(() => '')
        .tickSize(-config.bodyWidth)
        .tickFormat(() => '');
      horizontalGridLines(select(horizontalGridLineElement.current!));
    }, [config.itemCount, config.yScale, config.bodyWidth]);

    const verticalGridLineElement = useRef<SVGGElement | null>(null);
    useEffect(() => {
      const verticalGridLines = axisTop(config.xScale)
        .ticks(config.timeInterval)
        .tickSize(-config.itemCount * config.itemHeight)
        .tickFormat(() => '');
      verticalGridLines(select(verticalGridLineElement.current!));
    }, [
      config.itemCount,
      config.itemHeight,
      config.xScale,
      config.timeInterval,
    ]);

    const height = config.itemHeight * config.itemCount;
    return (
      <svg
        {...props}
        height={height}
        width={config.bodyWidth}
        ref={svgRef}
        onMouseOver={() => rulerStateSetters.setDisplay(true)}
        onMouseOut={() => rulerStateSetters.setDisplay(false)}
        onMouseMove={(e) => {
          const svgBox = svgRef.current!.getBoundingClientRect();
          const x = e.clientX - svgBox.x;
          rulerStateSetters.setX(x);
        }}
      >
        <g transform="translate(0.5, 0.5)">
          <g
            ref={horizontalGridLineElement}
            transform={`translate(-1, 0)`}
            css={{ '& line': { stroke: 'var(--divider-color)' } }}
          />
          <g
            ref={verticalGridLineElement}
            transform={`translate(0, -1)`}
            css={{ '& line': { stroke: 'var(--divider-color)' } }}
          />
          {children}
          <BodyRuler />
        </g>
        <path
          d={`m${config.bodyWidth - 0.5},0v${height + 2}`}
          stroke="currentcolor"
        />
      </svg>
    );
  },
);

export interface BodyProps {
  readonly content: (
    index: number,
    xScale: ScaleTime<number, number, never>,
  ) => ReactNode;
}

export function Body({ content }: BodyProps) {
  const config = useTimelineConfig();

  return (
    <Container width={config.bodyWidth}>
      <Virtuoso
        useWindowScroll
        components={{ Item: BodyItem, List: BodySvg }}
        totalCount={config.itemCount}
        fixedItemHeight={config.itemHeight}
        itemContent={(index) => {
          return (
            <g transform={`translate(0, ${(index + 0.5) * config.itemHeight})`}>
              {content(index, config.xScale)}
            </g>
          );
        }}
      />
    </Container>
  );
}
