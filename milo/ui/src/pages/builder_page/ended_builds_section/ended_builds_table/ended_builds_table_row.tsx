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
import { Box, Collapse, Icon, IconButton, Link, styled, TableCell, TableRow } from '@mui/material';
import { DateTime } from 'luxon';
import { observer } from 'mobx-react-lite';

import { Timestamp } from '../../../../components/timestamp';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP, BUILD_STATUS_ICON_MAP } from '../../../../libs/constants';
import { renderMarkdown } from '../../../../libs/markdown_utils';
import { displayDuration, NUMERIC_TIME_FORMAT } from '../../../../libs/time_utils';
import { getBuildURLPathFromBuildId, getGitilesCommitURL } from '../../../../libs/url_utils';
import { Build, getAssociatedGitilesCommit } from '../../../../services/buildbucket';
import { ExpandableEntriesStateInstance } from '../../../../store/expandable_entries_state';

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
    marginBlock: '10px',
  },
});

export interface EndedBuildsTableRowProps {
  readonly tableState: ExpandableEntriesStateInstance;
  readonly build: Build;
}

export const EndedBuildsTableRow = observer(({ tableState, build }: EndedBuildsTableRowProps) => {
  const expanded = Boolean(build.summaryMarkdown && tableState.isExpanded(build.id));

  const createTime = DateTime.fromISO(build.createTime);
  const startTime = build.startTime ? DateTime.fromISO(build.startTime) : null;
  const endTime = build.endTime ? DateTime.fromISO(build.endTime) : null;
  const runDuration = startTime && endTime ? endTime.diff(startTime) : null;
  const commit = getAssociatedGitilesCommit(build);

  return (
    <>
      <TableRow
        sx={{
          '& > td': { borderBottom: 'unset' },
        }}
      >
        <TableCell>
          {build.summaryMarkdown && (
            <IconButton aria-label="toggle-row" size="small" onClick={() => tableState.toggle(build.id, !expanded)}>
              {expanded ? <ExpandMore /> : <ChevronRight />}
            </IconButton>
          )}
        </TableCell>
        <TableCell>
          <link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Material+Icons&display=block" />
          <Icon className={BUILD_STATUS_CLASS_MAP[build.status]} title={BUILD_STATUS_DISPLAY_MAP[build.status]}>
            {BUILD_STATUS_ICON_MAP[build.status]}
          </Icon>
        </TableCell>
        <TableCell>
          <Link href={getBuildURLPathFromBuildId(build.id)}>{build.number ?? 'b' + build.id}</Link>
        </TableCell>
        <TableCell>
          <Timestamp datetime={createTime} format={NUMERIC_TIME_FORMAT} />
        </TableCell>
        <TableCell>{endTime ? <Timestamp datetime={endTime} format={NUMERIC_TIME_FORMAT} /> : 'N/A'}</TableCell>
        <TableCell>{runDuration ? displayDuration(runDuration) : 'N/A'}</TableCell>
        <TableCell>{commit ? <Link href={getGitilesCommitURL(commit)}>{commit.id}</Link> : 'N/A'}</TableCell>
      </TableRow>
      {build.summaryMarkdown && (
        // Change the component to `div` so CSS selector can skip this row when
        // selecting `<tr />`.
        <TableRow component="div" css={{ display: 'table-row' }}>
          <TableCell colSpan={7} sx={{ p: 0 }}>
            <Collapse in={expanded} timeout="auto">
              <MarkdownContainer
                className={`${BUILD_STATUS_CLASS_MAP[build.status]}-bg`}
                dangerouslySetInnerHTML={{ __html: renderMarkdown(build.summaryMarkdown || 'No Summary.') }}
              />
            </Collapse>
          </TableCell>
        </TableRow>
      )}
    </>
  );
});
