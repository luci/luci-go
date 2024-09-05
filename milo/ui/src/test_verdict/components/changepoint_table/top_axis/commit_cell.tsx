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

import { Box, Link, styled } from '@mui/material';
import { DateTime } from 'luxon';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { Timestamp } from '@/common/components/timestamp';
import { useVirtualizedQuery } from '@/generic_libs/hooks/virtualized_query';
import { ExtendedLogRequest } from '@/gitiles/api/fused_gitiles_client';
import { useGitilesClient } from '@/gitiles/hooks/prpc_client';
import { getGitilesCommitURL } from '@/gitiles/tools/utils';
import { OutputCommit } from '@/gitiles/types';
import { LogResponse } from '@/proto/go.chromium.org/luci/common/proto/gitiles/gitiles.pb';

import { LabelBox } from '../common';
import { CELL_WIDTH } from '../constants';
import { useConfig } from '../context';

const PAGE_SIZE = 200;

const CommitMetadata = styled(LabelBox)`
  width: ${CELL_WIDTH}px;

  & > * {
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: bold;
  }

  & > .commit-time {
    margin-top: 5px;
    text-wrap: pretty;
    color: ${(t) => t.theme.palette.text.disabled};
  }
`;

export interface CommitCellProps {
  readonly position: string;
}

export function CommitCell({ position }: CommitCellProps) {
  const {
    gitilesHost,
    gitilesRepository,
    gitilesRef,
    criticalCommits,
    rowHeight,
  } = useConfig();
  const lastCommitPosition = criticalCommits[0];
  const firstCommitPosition = criticalCommits[criticalCommits.length - 1];

  // We do not actually need a virtualized query here. The query is carefully
  // constructed such that it will hit the same cache as the query from the
  // `<BlamelistTable />`.  This allows the `<BlamelistTable />` to instantly
  // load the commits as long as user opens the blamelist around a commit
  // rendered by a commit cell.
  //
  // TODO: find a better way to ensure cache hit.
  const gitilesClient = useGitilesClient(gitilesHost);
  const gitilesQueries = useVirtualizedQuery({
    rangeBoundary: [
      -parseInt(lastCommitPosition),
      -parseInt(firstCommitPosition) + 1,
    ],
    interval: PAGE_SIZE,
    initRange: [-parseInt(position), -parseInt(position) + 1],
    genQuery: (start, end) => ({
      ...gitilesClient.ExtendedLog.query(
        ExtendedLogRequest.fromPartial({
          project: gitilesRepository,
          ref: gitilesRef,
          position: (-start).toString(),
          treeDiff: true,
          pageSize: end - start,
        }),
      ),
      // Commits are immutable.
      staleTime: Infinity,
      select: (data: LogResponse) => data.log as OutputCommit[],
    }),
  });
  const {
    data: commit,
    isError,
    error,
  } = gitilesQueries.get(-parseInt(position));

  if (isError) {
    throw error;
  }

  return (
    <HtmlTooltip
      placement="top"
      title={
        commit ? (
          <table>
            <tbody>
              <tr>
                <td colSpan={2}>
                  <strong>{commit.message.split('\n', 1)[0]}</strong>
                </td>
              </tr>
              <tr>
                <td width="1px" css={{ textWrap: 'nowrap' }}>
                  Author:
                </td>
                <td>{commit.author.name}</td>
              </tr>
              <tr>
                <td width="1px" css={{ textWrap: 'nowrap' }}>
                  Commit Time:
                </td>
                <td>
                  <Timestamp
                    datetime={DateTime.fromISO(commit.committer.time)}
                  />
                </td>
              </tr>
            </tbody>
          </table>
        ) : null
      }
    >
      <CommitMetadata sx={{ height: `${rowHeight}px` }}>
        <Box className="commit-position">
          <Link
            target="_blank"
            rel="noopenner"
            disabled={!commit}
            aria-disabled={!commit}
            tabIndex={commit ? undefined : -1}
            href={
              commit
                ? getGitilesCommitURL({
                    host: gitilesHost,
                    project: gitilesRepository,
                    id: commit.id,
                  })
                : ''
            }
          >
            {position}
          </Link>
        </Box>
        <Box className="commit-time">
          {commit ? (
            // Do not use a <Timestamp /> here. If we display a tooltip, it's
            // better to display more commit metadata on the tooltip.
            DateTime.fromISO(commit.committer.time).toFormat('MMM dd HH:mm')
          ) : (
            <></>
          )}
        </Box>
      </CommitMetadata>
    </HtmlTooltip>
  );
}
