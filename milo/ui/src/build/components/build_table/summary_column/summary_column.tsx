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
import { Box, IconButton, TableCell, styled } from '@mui/material';
import { computed } from 'mobx';
import { observer } from 'mobx-react-lite';
import { useMemo } from 'react';

import { BUILD_STATUS_CLASS_MAP } from '@/common/constants';
import { renderMarkdown } from '@/common/tools/markdown/utils';

import { useRowState, useTableState } from '../context';

export const SummaryHeadCell = observer(() => {
  const tableState = useTableState();

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
          onClick={() => tableState.toggleAll(!tableState.defaultExpanded)}
        >
          {tableState.defaultExpanded ? <ExpandMore /> : <ChevronRight />}
        </IconButton>
        <div css={{ lineHeight: '34px' }}>Summary</div>
      </div>
    </TableCell>
  );
});

const MarkdownContainer = styled(Box)({
  padding: '0 10px',
  clear: 'both',
  overflowWrap: 'break-word',
  '& pre': {
    whiteSpace: 'pre-wrap',
    overflowWrap: 'break-word',
    fontSize: '12px',
  },
  '& *': {
    marginBlock: '5px',
    paddingTop: '0',
    paddingBottom: '0',
    lineHeight: '20px',
  },
});

export const SummaryContentCell = observer(() => {
  const tableState = useTableState();
  const build = useRowState();

  const expandedObservable = useMemo(
    () =>
      computed(() => {
        // When there's no summary, always treat the cell as expanded so the
        // component doesn't need to be updated when the default expansion state
        // is updated.
        return !build.summaryMarkdown || tableState.isExpanded(build.id);
      }),
    [build, tableState]
  );
  const expanded = expandedObservable.get();

  const summaryHtml = useMemo(
    () =>
      build.summaryMarkdown
        ? renderMarkdown(build.summaryMarkdown)
        : '<p style="color: var(--greyed-out-text-color);">No Summary.</p>',
    [build.summaryMarkdown]
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
            onClick={() => tableState.toggle(build.id, !expanded)}
            // Always render the button to DOM so we have a stable layout.
            // Hide it from users so it won't mislead users to think there
            // are more summary.
            disabled={!build.summaryMarkdown}
            sx={{ visibility: build.summaryMarkdown ? '' : 'hidden' }}
          >
            {expanded ? <ExpandMore /> : <ChevronRight />}
          </IconButton>
        </div>
        <MarkdownContainer
          className={`${BUILD_STATUS_CLASS_MAP[build.status]}-bg`}
          css={{
            marginBottom: '2px',
            ...(expanded
              ? {}
              : {
                  // Cap the size of the markdown container to only show the
                  // first line.
                  height: '30px',
                  overflow: 'hidden',
                  // Use dashed bottom border to hint that there could be
                  // more summary.
                  borderBottomStyle: 'dashed',
                }),
          }}
          dangerouslySetInnerHTML={{
            __html: summaryHtml,
          }}
        />
      </div>
    </TableCell>
  );
});