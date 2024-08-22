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

import { UseQueryOptions, useQueries } from '@tanstack/react-query';
import { debounce } from 'lodash-es';
import { useState } from 'react';

import { BatchedClustersClientProvider } from '@/analysis/hooks/bached_clusters_client/context';
import { useTestVariantBranchesClient } from '@/analysis/hooks/prpc_clients';
import {
  OutputQuerySourcePositionsResponse,
  OutputTestVariantBranch,
} from '@/analysis/types';
import {
  AuthorContentCell,
  AuthorHeadCell,
  CommitTableRow,
  PositionContentCell,
  PositionHeadCell,
  TimeContentCell,
  TimeHeadCell,
  TitleContentCell,
  TitleHeadCell,
  ToggleContentCell,
  ToggleHeadCell,
  VirtualizedCommitTable,
} from '@/gitiles/components/commit_table';
import { getGitilesRepoURL } from '@/gitiles/tools/utils';
import {
  QuerySourcePositionsRequest,
  QuerySourcePositionsResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { EntryContent } from './entry_content';
import { BlamelistContextProvider } from './context';
import { SegmentContentCell, SegmentHeadCell } from './segment_column';
import {
  VerdictsStatusHeadCell,
  VerdictStatusesContentCell,
} from './verdicts_status_column';

const PAGE_SIZE = 1000;

function getOffset(lastPosition: string, position: string) {
  return parseInt(lastPosition) - parseInt(position);
}

function getPosition(lastPosition: string, offset: number) {
  return (parseInt(lastPosition) - offset).toString();
}

export interface BlamelistTable {
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly focusCommitPosition?: string | null;
  readonly customScrollParent?: HTMLElement;
}

export function BlamelistTable({
  testVariantBranch,
  customScrollParent,
  focusCommitPosition,
}: BlamelistTable) {
  const [pageStart, setPageStart] = useState(0);
  const [pageEnd, setPageEnd] = useState(0);
  const [handleRangeChanged] = useState(() =>
    debounce((itemStart: number, itemEnd: number) => {
      setPageStart(Math.floor(itemStart / PAGE_SIZE));
      setPageEnd(Math.ceil(itemEnd / PAGE_SIZE));
    }, 500),
  );

  const segments = testVariantBranch.segments;
  const lastPosition = segments[0].endPosition;
  const firstPosition = segments[segments.length - 1].startPosition;

  const client = useTestVariantBranchesClient();
  type QueryOpts = UseQueryOptions<
    QuerySourcePositionsResponse,
    unknown,
    OutputQuerySourcePositionsResponse
  >;
  const queries = useQueries({
    queries: Array(pageEnd - pageStart)
      .fill(undefined)
      .map<QueryOpts>((_, i) => ({
        ...client.QuerySourcePositions.query(
          QuerySourcePositionsRequest.fromPartial({
            project: testVariantBranch.project,
            testId: testVariantBranch.testId,
            variantHash: testVariantBranch.variantHash,
            refHash: testVariantBranch.refHash,
            startSourcePosition: getPosition(
              lastPosition,
              (pageStart + i) * PAGE_SIZE,
            ),
            pageSize: PAGE_SIZE,
          }),
        ),
        select: (data) => data as OutputQuerySourcePositionsResponse,
        // The query is expensive and the blamelist should be stable anyway.
        refetchOnWindowFocus: false,
        staleTime: Infinity,
      })),
  });
  for (const { isError, error } of queries) {
    if (isError) {
      throw error;
    }
  }

  return (
    <BlamelistContextProvider testVariantBranch={testVariantBranch}>
      <BatchedClustersClientProvider>
        <VirtualizedCommitTable
          repoUrl={getGitilesRepoURL(testVariantBranch.ref.gitiles)}
          customScrollParent={customScrollParent}
          rangeChanged={(r) => handleRangeChanged(r.startIndex, r.endIndex + 1)}
          increaseViewportBy={1000}
          initialTopMostItemIndex={
            focusCommitPosition
              ? getOffset(lastPosition, focusCommitPosition)
              : 0
          }
          totalCount={getOffset(lastPosition, firstPosition) + 1}
          fixedHeaderContent={() => (
            <>
              <SegmentHeadCell />
              <ToggleHeadCell hotkey="x" />
              <VerdictsStatusHeadCell />
              <PositionHeadCell />
              <TimeHeadCell />
              <AuthorHeadCell />
              <TitleHeadCell />
            </>
          )}
          itemContent={(i) => {
            const position = getPosition(lastPosition, i);
            const page = queries[Math.floor(i / PAGE_SIZE) - pageStart]?.data;
            const sp = page?.sourcePositions[i % PAGE_SIZE];

            return (
              <CommitTableRow
                commit={sp?.commit || null}
                content={<EntryContent verdicts={sp?.verdicts || null} />}
              >
                <SegmentContentCell position={position} />
                <ToggleContentCell />
                <VerdictStatusesContentCell
                  testVerdicts={sp?.verdicts || null}
                />
                <PositionContentCell position={position} />
                <TimeContentCell />
                <AuthorContentCell />
                <TitleContentCell />
              </CommitTableRow>
            );
          }}
          sx={{ '& td:last-of-type': { flexGrow: 0 } }}
        />
      </BatchedClustersClientProvider>
    </BlamelistContextProvider>
  );
}
