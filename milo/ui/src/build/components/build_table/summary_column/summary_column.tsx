// Copyright 2023 The LUCI Authors.
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

import { ChevronRight, ExpandMore } from '@mui/icons-material';
import { IconButton, TableCell, styled } from '@mui/material';
import { useMemo } from 'react';

import { SpecifiedBuildStatus } from '@/build/types';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { BUILD_STATUS_CLASS_MAP } from '@/common/constants/build';
import { renderMarkdown } from '@/common/tools/markdown/utils';

import {
  useBuild,
  useDefaultExpanded,
  useSetDefaultExpanded,
  useSetRowExpanded,
} from '../context';

export function SummaryHeadCell() {
  const defaultExpanded = useDefaultExpanded();
  const setDefaultExpanded = useSetDefaultExpanded();

  return (
    <TableCell>
      <div
        css={{
          display: 'grid',
          gridTemplateColumns: '34px 1fr',
        }}
      >
        <IconButton
          aria-label="toggle-all-rows"
          size="small"
          onClick={() => setDefaultExpanded(!defaultExpanded)}
        >
          {defaultExpanded ? <ExpandMore /> : <ChevronRight />}
        </IconButton>
        <div css={{ lineHeight: '34px' }}>Summary</div>
      </div>
    </TableCell>
  );
}

const SummaryContainer = styled(SanitizedHtml)({
  marginBottom: '2px',
  padding: '0 10px',
  clear: 'both',
  overflowWrap: 'break-word',
  whiteSpace: 'normal',
  '& pre': {
    whiteSpace: 'pre-wrap',
    overflowWrap: 'break-word',
    fontSize: '12px',
  },
  // The following two blocks of rules are needed to ensure the first line of
  // the summary always have the same line height and margin/padding. This
  // allows us to use them as short summaries when the summary is collapsed.
  '& *': {
    marginBlock: '0px',
    paddingTop: '0',
    paddingBottom: '0',
    lineHeight: '20px',
  },
  '& > *': {
    marginBlock: '5px',
  },
  '.BuildTableRow-collapsed &': {
    // Cap the size of the markdown container to only show the
    // first line.
    height: '30px',
    overflow: 'hidden',
    // Ensure the 2nd line isn't partially rendered in the box
    // causing visual noises.
    fontSize: 0,
    '&::first-line': {
      fontSize: '0.875rem',
    },
    // Use dashed bottom border to hint that there could be
    // more summary.
    borderBottomStyle: 'dashed',
  },
});

export function SummaryContentCell() {
  const setExpanded = useSetRowExpanded();
  const build = useBuild();

  const summaryHtml = useMemo(
    () =>
      build.summaryMarkdown
        ? renderMarkdown(build.summaryMarkdown)
        : '<p style="color: var(--greyed-out-text-color);">No Summary.</p>',
    [build.summaryMarkdown],
  );

  return (
    <TableCell>
      <div
        css={{
          display: 'grid',
          gridTemplateColumns: '34px 1fr',
        }}
      >
        <div>
          <IconButton
            aria-label="toggle-row"
            size="small"
            onClick={() => setExpanded((expanded) => !expanded)}
            // Always render the button to DOM so we have a stable layout.
            // Hide it from users so it won't mislead users to think there
            // are more summary.
            disabled={!build.summaryMarkdown}
            sx={{ visibility: build.summaryMarkdown ? '' : 'hidden' }}
          >
            <ExpandMore
              sx={{ '.BuildTableRow-collapsed &': { display: 'none' } }}
            />
            <ChevronRight
              sx={{ '.BuildTableRow-expanded &': { display: 'none' } }}
            />
          </IconButton>
        </div>
        <SummaryContainer
          className={`${
            BUILD_STATUS_CLASS_MAP[build.status as SpecifiedBuildStatus]
          }-bg`}
          html={summaryHtml}
        />
      </div>
    </TableCell>
  );
}
