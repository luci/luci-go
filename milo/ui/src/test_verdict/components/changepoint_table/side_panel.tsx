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
import { axisLeft, select } from 'd3';
import { useEffect, useMemo, useRef } from 'react';

import {
  ParsedTestVariantBranchName,
  TestVariantBranchDef,
} from '@/analysis/types';
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import { getLongestCommonPrefix } from '@/generic_libs/tools/string_utils';

import { LabelBox } from './common';
import { SIDE_PANEL_WIDTH } from './constants';
import { useConfig } from './hooks';

const CopyableLabelBox = styled(LabelBox)`
  display: grid;
  grid-template-columns: auto auto 1fr;
  &:not(:hover) > .show-on-hover {
    display: none;
  }
`;

export interface SidePanelProps {
  readonly testVariantBranches: readonly TestVariantBranchDef[];
}

export function SidePanel({ testVariantBranches }: SidePanelProps) {
  const { criticalVariantKeys, yScale, testVariantBranchCount, rowHeight } =
    useConfig();

  const gridLineElement = useRef<SVGGElement>(null);
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
        border: 'solid 1px var(--divider-color)',
      }}
      height={height}
      width={SIDE_PANEL_WIDTH}
    >
      <g
        ref={gridLineElement}
        css={{
          '& line,path': { stroke: 'var(--divider-color)' },
        }}
        transform="translate(-1, 0)"
      />
      {testVariantBranches.map((tvb, i) => (
        <g
          key={ParsedTestVariantBranchName.toString(tvb)}
          transform={`translate(0, ${yScale(i)})`}
        >
          <foreignObject
            height={rowHeight}
            width={SIDE_PANEL_WIDTH}
            css={{ position: 'relative' }}
          >
            <Box
              sx={{
                top: '50%',
                transform: 'translateY(-50%)',
                position: 'relative',
              }}
            >
              <CopyableLabelBox title={tvb.testId}>
                <Box
                  css={{
                    textOverflow: 'ellipsis',
                    overflow: 'hidden',
                    fontWeight: 'bold',
                  }}
                >
                  {commonPrefix && '...'}
                  {tvb.testId.slice(commonPrefix.length)}
                </Box>
                <CopyToClipboard
                  textToCopy={tvb.testId}
                  sx={{ verticalAlign: 'middle' }}
                  className="show-on-hover"
                />
              </CopyableLabelBox>
              {criticalVariantKeys.map((k) => (
                <CopyableLabelBox key={k}>
                  <Box css={{ textOverflow: 'ellipsis', overflow: 'hidden' }}>
                    {tvb.variant?.def[k]}
                  </Box>
                  <CopyToClipboard
                    textToCopy={tvb.variant?.def[k] || ''}
                    sx={{ verticalAlign: 'middle' }}
                    className="show-on-hover"
                  />
                </CopyableLabelBox>
              ))}
            </Box>
          </foreignObject>
        </g>
      ))}
    </svg>
  );
}
