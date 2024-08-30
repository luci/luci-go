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
  OutputTestVariantBranch,
  ParsedTestVariantBranchName,
} from '@/analysis/types';
import { ExtendedLogRequest } from '@/gitiles/api/fused_gitiles_client';
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
import { useGitilesClient } from '@/gitiles/hooks/prpc_client';
import { getGitilesRepoURL } from '@/gitiles/tools/utils';
import { OutputLogResponse } from '@/gitiles/types';
import {
  QuerySourceVerdictsRequest,
  QuerySourceVerdictsResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { LogResponse } from '@/proto/go.chromium.org/luci/common/proto/gitiles/gitiles.pb';

import { BlamelistContextProvider } from './context';
import { EntryContent } from './entry_content';
import { SegmentContentCell, SegmentHeadCell } from './segment_column';
import {
  SourceVerdictStatusHeadCell,
  SourceVerdictStatusContentCell,
} from './source_verdict_status_column';

const PAGE_SIZE = 1000;

function getOffset(lastPosition: string, position: string) {
  return parseInt(lastPosition) - parseInt(position);
}

function getPosition(lastPosition: string, offset: number) {
  return (parseInt(lastPosition) - offset).toString();
}

export interface BlamelistTable {
  readonly lastCommitPosition: string;
  readonly firstCommitPosition: string;
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly focusCommitPosition?: string | null;
  readonly customScrollParent?: HTMLElement;
}

export function BlamelistTable({
  lastCommitPosition,
  firstCommitPosition,
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

  const gitilesClient = useGitilesClient(testVariantBranch.ref.gitiles.host);
  type GitilesQueryOpts = UseQueryOptions<
    LogResponse,
    unknown,
    OutputLogResponse
  >;
  const gitilesQueries = useQueries({
    queries: Array(pageEnd - pageStart)
      .fill(undefined)
      .map<GitilesQueryOpts>((_, i) => ({
        ...gitilesClient.ExtendedLog.query(
          ExtendedLogRequest.fromPartial({
            project: testVariantBranch.ref.gitiles.project,
            ref: testVariantBranch.ref.gitiles.ref,
            position: getPosition(
              lastCommitPosition,
              (pageStart + i) * PAGE_SIZE,
            ),
            treeDiff: true,
            pageSize: PAGE_SIZE,
          }),
        ),
        // Commits are immutable.
        staleTime: Infinity,
        select: (data) => data as OutputLogResponse,
      })),
  });
  for (const { isError, error } of gitilesQueries) {
    if (isError) {
      throw error;
    }
  }

  const tvbClient = useTestVariantBranchesClient();
  type VerdictsQueryOpts = UseQueryOptions<QuerySourceVerdictsResponse>;
  const verdictQueries = useQueries({
    queries: Array(pageEnd - pageStart)
      .fill(undefined)
      .map<VerdictsQueryOpts>((_, i) => ({
        ...tvbClient.QuerySourceVerdicts.query(
          QuerySourceVerdictsRequest.fromPartial({
            parent: ParsedTestVariantBranchName.toString(testVariantBranch),
            startSourcePosition: getPosition(
              lastCommitPosition,
              (pageStart + i) * PAGE_SIZE,
            ),
            endSourcePosition: getPosition(
              lastCommitPosition,
              (pageStart + i + 1) * PAGE_SIZE,
            ),
          }),
        ),
      })),
  });
  for (const { isError, error } of verdictQueries) {
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
              ? getOffset(lastCommitPosition, focusCommitPosition)
              : 0
          }
          totalCount={getOffset(lastCommitPosition, firstCommitPosition) + 1}
          fixedHeaderContent={() => (
            <>
              <SegmentHeadCell />
              <ToggleHeadCell hotkey="x" />
              <SourceVerdictStatusHeadCell />
              <PositionHeadCell />
              <TimeHeadCell />
              <AuthorHeadCell />
              <TitleHeadCell />
            </>
          )}
          itemContent={(i) => {
            const position = getPosition(lastCommitPosition, i);
            const pageIndex = Math.floor(i / PAGE_SIZE) - pageStart;
            const pageOffset = i % PAGE_SIZE;
            const commit = gitilesQueries[pageIndex]?.data?.log[pageOffset];
            const verdictPage = verdictQueries[pageIndex]?.data;
            const sourceVerdict = verdictPage?.sourceVerdicts.find(
              (sv) => sv.position === position,
            );

            return (
              <CommitTableRow
                commit={commit || null}
                content={
                  <EntryContent
                    testId={testVariantBranch.testId}
                    variantHash={testVariantBranch.variantHash}
                    sourceVerdict={sourceVerdict || null}
                    isSvLoading={!verdictPage}
                  />
                }
              >
                <SegmentContentCell position={position} />
                <ToggleContentCell />
                <SourceVerdictStatusContentCell
                  status={sourceVerdict?.status || null}
                  isLoading={!verdictPage}
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
