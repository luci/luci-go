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

import { ListRange } from 'react-virtuoso';

import { useTestVariantBranchesClient } from '@/analysis/hooks/prpc_clients';
import {
  OutputTestVariantBranch,
  ParsedTestVariantBranchName,
} from '@/analysis/types';
import {
  UseVirtualizedQueryOption,
  useVirtualizedQuery,
} from '@/generic_libs/hooks/virtualized_query';
import { ExtendedLogRequest } from '@/gitiles/api/fused_gitiles_client';
import {
  VirtualizedCommitTable,
  VirtualizedCommitTableProps,
} from '@/gitiles/components/commit_table';
import { useGitilesClient } from '@/gitiles/hooks/prpc_client';
import { getGitilesRepoURL } from '@/gitiles/tools/utils';
import { OutputCommit } from '@/gitiles/types';
import {
  QuerySourceVerdictsRequest,
  QuerySourceVerdictsResponse,
  QuerySourceVerdictsResponse_SourceVerdict,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { LogResponse } from '@/proto/go.chromium.org/luci/common/proto/gitiles/gitiles.pb';

import { BlamelistContextProvider } from './context';
import { BlamelistTableRow, BlamelistTableHeaderContent } from './row';

const PAGE_SIZE = 200;
const DELAY_MS = 500;

function getOffset(lastPosition: string, position: string) {
  return parseInt(lastPosition) - parseInt(position);
}

function getPosition(lastPosition: string, offset: number) {
  return (parseInt(lastPosition) - offset).toString();
}

export interface BlamelistTableProps
  extends Omit<
    VirtualizedCommitTableProps,
    | 'repoUrl'
    | 'rangeChanged'
    | 'initialTopMostItemIndex'
    | 'totalCount'
    | 'fixedHeaderContent'
    | 'itemContent'
    | 'sx'
  > {
  readonly lastCommitPosition: string;
  readonly firstCommitPosition: string;
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly focusCommitPosition?: string;
}

export function BlamelistTable({
  lastCommitPosition,
  firstCommitPosition,
  testVariantBranch,
  focusCommitPosition = lastCommitPosition,
  ...props
}: BlamelistTableProps) {
  // Note that we use a negative index so commits are sorted by their commit
  // position in descending order.
  //
  // When the gitiles query is updated, also ensure that
  // `@/test_verdict/components/changepoint_table/top_axis/commit_cell` can hits
  // the same cache.
  const queryOptsBase: Omit<
    UseVirtualizedQueryOption<unknown, unknown>,
    'genQuery'
  > = {
    rangeBoundary: [
      -parseInt(lastCommitPosition),
      -parseInt(firstCommitPosition) + 1,
    ],
    interval: PAGE_SIZE,
    initRange: [
      -parseInt(focusCommitPosition),
      -parseInt(focusCommitPosition) + 1,
    ],
    delayMs: DELAY_MS,
  };
  const gitilesClient = useGitilesClient(testVariantBranch.ref.gitiles.host);
  const gitilesQueries = useVirtualizedQuery({
    ...queryOptsBase,
    genQuery: (start, end) => ({
      ...gitilesClient.ExtendedLog.query(
        ExtendedLogRequest.fromPartial({
          project: testVariantBranch.ref.gitiles.project,
          ref: testVariantBranch.ref.gitiles.ref,
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

  const tvbClient = useTestVariantBranchesClient();
  const verdictQueries = useVirtualizedQuery({
    ...queryOptsBase,
    genQuery: (start, end) => ({
      ...tvbClient.QuerySourceVerdicts.query(
        QuerySourceVerdictsRequest.fromPartial({
          parent: ParsedTestVariantBranchName.toString(testVariantBranch),
          startSourcePosition: (-start).toString(),
          endSourcePosition: (-end).toString(),
        }),
      ),
      // QuerySourceVerdicts is somewhat expensive.
      // Do not refresh SVs associated with commits older than 1000 commits.
      // For SVs associated with newer commits, do not refresh if the SVs are
      // queried less than 5 mins ago.
      staleTime:
        start - queryOptsBase.rangeBoundary[0] > 1000 ? Infinity : 300_000,
      select: (data: QuerySourceVerdictsResponse) => {
        const svs: Array<
          QuerySourceVerdictsResponse_SourceVerdict | undefined
        > = [];
        for (const sv of data.sourceVerdicts) {
          svs[-start - parseInt(sv.position)] = sv;
        }
        return svs;
      },
    }),
  });

  function handleRangeChanged(range: ListRange) {
    const mappedRange = [
      -parseInt(getPosition(lastCommitPosition, range.startIndex)),
      -parseInt(getPosition(lastCommitPosition, range.endIndex)) + 1,
    ] as const;
    gitilesQueries.setRange(mappedRange);
    verdictQueries.setRange(mappedRange);
  }

  return (
    <BlamelistContextProvider testVariantBranch={testVariantBranch}>
      <VirtualizedCommitTable
        increaseViewportBy={1000}
        {...props}
        repoUrl={getGitilesRepoURL(testVariantBranch.ref.gitiles)}
        rangeChanged={handleRangeChanged}
        initialTopMostItemIndex={getOffset(
          lastCommitPosition,
          focusCommitPosition,
        )}
        totalCount={getOffset(lastCommitPosition, firstCommitPosition) + 1}
        fixedHeaderContent={() => <BlamelistTableHeaderContent />}
        itemContent={(i) => {
          const position = getPosition(lastCommitPosition, i);
          const cpIndex = -parseInt(position);
          const commitQuery = gitilesQueries.get(cpIndex);
          if (commitQuery.isError) {
            throw commitQuery.error;
          }

          const sourceVerdictQuery = verdictQueries.get(cpIndex);
          if (sourceVerdictQuery.isError) {
            throw sourceVerdictQuery.error;
          }

          return (
            <BlamelistTableRow
              commit={commitQuery.data || null}
              position={position}
              testVariantBranch={testVariantBranch}
              sourceVerdict={sourceVerdictQuery.data || null}
              isSvLoading={sourceVerdictQuery.isLoading}
            />
          );
        }}
        sx={{ '& td:last-of-type': { flexGrow: 0 } }}
      />
    </BlamelistContextProvider>
  );
}
