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

import { Box, CircularProgress } from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { BatchedClustersClientProvider } from '@/analysis/hooks/batched_clusters_client';
import { useTestVariantBranchesClient } from '@/analysis/hooks/prpc_clients';
import {
  OutputSourcePosition,
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
import { QuerySourcePositionsRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { BlamelistContextProvider } from './context';
import { EntryContent } from './entry_content';
import { LoadingRow } from './loading_row';
import { SegmentContentCell, SegmentHeadCell } from './segment_column';
import {
  VerdictsStatusHeadCell,
  VerdictStatusesContentCell,
} from './verdicts_status_column';

export interface BlamelistTable {
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly customScrollParent?: HTMLElement;
}

export function BlamelistTable({
  testVariantBranch,
  customScrollParent,
}: BlamelistTable) {
  const client = useTestVariantBranchesClient();
  const { data, isLoading, isError, error, hasNextPage, fetchNextPage } =
    useInfiniteQuery({
      ...client.QuerySourcePositions.queryPaged(
        QuerySourcePositionsRequest.fromPartial({
          project: testVariantBranch.project,
          testId: testVariantBranch.testId,
          variantHash: testVariantBranch.variantHash,
          refHash: testVariantBranch.refHash,
          startSourcePosition: testVariantBranch.segments[0].endPosition,
          pageSize: 1000,
        }),
      ),
    });
  if (isError) {
    throw error;
  }

  const nextCommitPosition = useMemo(() => {
    if (!data || !hasNextPage) {
      return null;
    }
    for (let i = data.pages.length - 1; i >= 0; i--) {
      const sps = data.pages[i].sourcePositions;
      if (sps.length === 0) {
        continue;
      }
      const lastPosition = sps[sps.length - 1].position;
      return (parseInt(lastPosition) + 1).toString();
    }
    return null;
  }, [data, hasNextPage]);
  const repoUrl = getGitilesRepoURL(testVariantBranch.ref.gitiles);
  const sourcePositions = useMemo(
    () =>
      data?.pages.flatMap((p) => p.sourcePositions as OutputSourcePosition[]) ||
      [],
    [data?.pages],
  );

  return (
    <BlamelistContextProvider testVariantBranch={testVariantBranch}>
      <BatchedClustersClientProvider>
        {isLoading ? (
          <Box display="flex" justifyContent="center" alignItems="center">
            <CircularProgress />
          </Box>
        ) : (
          <VirtualizedCommitTable
            customScrollParent={customScrollParent}
            repoUrl={repoUrl}
            totalCount={sourcePositions.length + (hasNextPage ? 1 : 0)}
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
              if (i === sourcePositions.length) {
                return (
                  <LoadingRow
                    loadedPageCount={data.pages.length}
                    nextCommitPosition={nextCommitPosition}
                    loadNextPage={() => fetchNextPage()}
                  />
                );
              }

              const sp = sourcePositions[i];
              return (
                <CommitTableRow
                  key={sp.position}
                  commit={sp.commit}
                  content={<EntryContent verdicts={sp.verdicts} />}
                >
                  <SegmentContentCell position={sp.position} />
                  <ToggleContentCell />
                  <VerdictStatusesContentCell testVerdicts={sp.verdicts} />
                  <PositionContentCell position={sp.position} />
                  <TimeContentCell />
                  <AuthorContentCell />
                  <TitleContentCell />
                </CommitTableRow>
              );
            }}
            sx={{ '& td:last-of-type': { flexGrow: 0 } }}
          />
        )}
      </BatchedClustersClientProvider>
    </BlamelistContextProvider>
  );
}
