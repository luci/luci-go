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
import {
  Box,
  Collapse,
  Icon,
  IconButton,
  Link,
  styled,
  TableCell,
  TableRow,
} from '@mui/material';
import { DateTime } from 'luxon';
import { observer } from 'mobx-react-lite';
import { Fragment } from 'react';

import {
  DEFAULT_EXTRA_ZONE_CONFIGS,
  Timestamp,
} from '@/common/components/timestamp';
import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_DISPLAY_MAP,
  BUILD_STATUS_ICON_MAP,
} from '@/common/constants';
import {
  Build,
  getAssociatedGitilesCommit,
} from '@/common/services/buildbucket';
import { ExpandableEntriesStateInstance } from '@/common/store/expandable_entries_state';
import {
  getGitilesCommitLabel,
  getGitilesCommitURL,
} from '@/common/tools/gitiles_utils';
import { renderMarkdown } from '@/common/tools/markdown/utils';
import {
  NUMERIC_TIME_FORMAT,
  SHORT_TIME_FORMAT,
  displayDuration,
} from '@/common/tools/time_utils';
import {
  getBuildURLPathFromBuildId,
  getGerritChangeURL,
} from '@/common/tools/url_utils';

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

function CompactTimestamp({ datetime }: { datetime: DateTime }) {
  return (
    <Timestamp
      datetime={datetime}
      // Use a more compact format to diaply the timestamp.
      format={SHORT_TIME_FORMAT}
      extra={{
        // Use a more detailed format in the tooltip.
        format: NUMERIC_TIME_FORMAT,
        zones: [
          // Add a local timezone to display the timestamp in local timezone
          // with a more detailed format.
          {
            label: 'LOCAL',
            zone: 'local',
          },
          ...DEFAULT_EXTRA_ZONE_CONFIGS,
        ],
      }}
    />
  );
}

export interface EndedBuildsTableRowProps {
  readonly tableState: ExpandableEntriesStateInstance;
  readonly build: Build;
  readonly displayGerritChanges: boolean;
}

export const EndedBuildsTableRow = observer(
  ({ tableState, build, displayGerritChanges }: EndedBuildsTableRowProps) => {
    const expanded = tableState.isExpanded(build.id);

    const createTime = DateTime.fromISO(build.createTime);
    const startTime = build.startTime
      ? DateTime.fromISO(build.startTime)
      : null;
    const endTime = build.endTime ? DateTime.fromISO(build.endTime) : null;
    const runDuration = startTime && endTime ? endTime.diff(startTime) : null;
    const commit = getAssociatedGitilesCommit(build);
    const changes = build.input?.gerritChanges || [];

    return (
      <>
        <TableRow
          sx={{
            '& > td': { borderBottom: 'unset' },
          }}
        >
          <TableCell>
            <IconButton
              aria-label="toggle-row"
              size="small"
              onClick={() => tableState.toggle(build.id, !expanded)}
            >
              {expanded ? <ExpandMore /> : <ChevronRight />}
            </IconButton>
          </TableCell>
          <TableCell>
            <link
              rel="stylesheet"
              href="https://fonts.googleapis.com/css?family=Material+Icons&display=block"
            />
            <Icon
              className={BUILD_STATUS_CLASS_MAP[build.status]}
              title={BUILD_STATUS_DISPLAY_MAP[build.status]}
            >
              {BUILD_STATUS_ICON_MAP[build.status]}
            </Icon>
          </TableCell>
          <TableCell>
            <Link href={getBuildURLPathFromBuildId(build.id)}>
              {build.number ?? 'b' + build.id}
            </Link>
          </TableCell>
          <TableCell>
            <CompactTimestamp datetime={createTime} />
          </TableCell>
          <TableCell>
            {endTime ? <CompactTimestamp datetime={endTime} /> : 'N/A'}
          </TableCell>
          <TableCell>
            {runDuration ? displayDuration(runDuration) : 'N/A'}
          </TableCell>
          <TableCell>
            {commit ? (
              <Link href={getGitilesCommitURL(commit)}>
                {getGitilesCommitLabel(commit)}
              </Link>
            ) : (
              'N/A'
            )}
          </TableCell>
          {displayGerritChanges && (
            <TableCell>
              {changes.map((c, i) => (
                <Fragment key={c.change}>
                  {i !== 0 && <>, </>}
                  <Link key={c.change} href={getGerritChangeURL(c)}>
                    CL {c.change} (ps #{c.patchset})
                  </Link>
                </Fragment>
              ))}
            </TableCell>
          )}
        </TableRow>
        <TableRow>
          <TableCell colSpan={8} sx={{ p: 0 }}>
            <Collapse in={expanded} timeout="auto">
              <MarkdownContainer
                className={`${BUILD_STATUS_CLASS_MAP[build.status]}-bg`}
                dangerouslySetInnerHTML={{
                  __html: renderMarkdown(
                    build.summaryMarkdown || 'No Summary.'
                  ),
                }}
              />
            </Collapse>
          </TableCell>
        </TableRow>
      </>
    );
  }
);
