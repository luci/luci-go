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

import { Box, styled, useTheme } from '@mui/material';
import { axisLeft, select } from 'd3';
import { ReactNode, forwardRef, useEffect, useRef } from 'react';
import { Virtuoso } from 'react-virtuoso';

import { useTimelineConfig } from './context';

interface OptionalChildrenProps {
  readonly children?: ReactNode;
}

function SidePanelItem({ ...props }: OptionalChildrenProps) {
  return <g {...props} />;
}

const SidePanelSvg = forwardRef<HTMLDivElement, OptionalChildrenProps>(
  function SidePanelSvg({ children, ...props }, _ref) {
    const config = useTimelineConfig();

    const gridLineElement = useRef<SVGGElement | null>(null);
    useEffect(() => {
      const horizontalGridLines = axisLeft(config.yScale)
        .ticks(config.itemCount)
        .tickFormat(() => '')
        .tickSize(-config.sidePanelWidth)
        .tickFormat(() => '');
      horizontalGridLines(select(gridLineElement.current!));
    }, [config.yScale, config.itemCount, config.sidePanelWidth]);
    const height = config.itemHeight * config.itemCount;

    return (
      <svg {...props} height={height} width={config.sidePanelWidth}>
        <g
          ref={gridLineElement}
          transform={`translate(0.5, 0.5)`}
          css={{ '& line': { stroke: 'var(--divider-color)' } }}
        />
        {children}
        <path
          d={`m0.5,-1v${height + 2}m${config.sidePanelWidth - 1},0v${
            -height - 2
          }`}
          stroke="currentcolor"
        />
      </svg>
    );
  },
);

const Container = styled(Box)`
  grid-area: side-panel;
  position: sticky;
  left: 0;
  z-index: 1;
`;

export interface SidePanelProps {
  readonly content: (index: number) => ReactNode;
}

export function SidePanel({ content }: SidePanelProps) {
  const theme = useTheme();
  const config = useTimelineConfig();

  return (
    <Container
      sx={{
        backgroundColor: theme.palette.background.default,
        width: config.sidePanelWidth,
      }}
    >
      <Virtuoso
        useWindowScroll
        components={{ Item: SidePanelItem, List: SidePanelSvg }}
        totalCount={config.itemCount}
        fixedItemHeight={config.itemHeight}
        itemContent={(index) => {
          return (
            <g transform={`translate(0, ${(index + 0.5) * config.itemHeight})`}>
              {content(index)}
            </g>
          );
        }}
      />
    </Container>
  );
}