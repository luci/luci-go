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

import { Button, CircularProgress } from '@mui/material';
import { InfiniteData, useInfiniteQuery } from '@tanstack/react-query';

import { useMiloInternalClient } from '@/common/hooks/prpc_clients';
import { getGitilesRepoURL } from '@/gitiles/tools/utils';
import { Analysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { QueryBlamelistRequest } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

import { BlamelistTable } from './blamelist_table';
import { RevisionRange } from './revision_range';
import { OutputQueryBlamelistResponse } from './types';

export interface BlamelistDisplayProps {
  readonly blamelistPin: GitilesCommit;
  readonly builder: BuilderID;
  readonly analysis?: Analysis;
}

export function BlamelistDisplay({
  blamelistPin,
  builder,
  analysis,
}: BlamelistDisplayProps) {
  const client = useMiloInternalClient();
  const {
    data,
    error,
    isError,
    hasNextPage,
    fetchNextPage,
    isPending,
    isFetchingNextPage,
  } = useInfiniteQuery({
    ...client.QueryBlamelist.queryPaged(
      QueryBlamelistRequest.fromPartial({
        gitilesCommit: blamelistPin,
        builder,
      }),
    ),
    // The query is expensive and the blamelist should be stable anyway.
    refetchOnWindowFocus: false,
    select: (data) => data as InfiniteData<OutputQueryBlamelistResponse>,
  });

  if (isError) {
    throw error;
  }

  const pages = data?.pages || [];
  const isLoadingPage = isPending || isFetchingNextPage;
  const commitCount = pages.reduce(
    (prev, page) => prev + page.commits.length,
    0,
  );

  return (
    <>
      <div css={{ padding: '5px 10px' }}>
        <RevisionRange
          blamelistPin={blamelistPin}
          commitCount={commitCount}
          precedingCommit={
            !hasNextPage && pages.length
              ? pages[pages.length - 1].precedingCommit
              : undefined
          }
        />
      </div>
      <BlamelistTable
        repoUrl={getGitilesRepoURL(blamelistPin)}
        pages={pages}
        analysis={analysis}
      />
      <div css={{ padding: '5px 10px' }}>
        <Button
          disabled={isLoadingPage || !hasNextPage}
          onClick={() => fetchNextPage()}
          endIcon={isLoadingPage ? <CircularProgress size={15} /> : <></>}
        >
          {isLoadingPage ? 'Loading' : 'Load more'}
        </Button>
      </div>
    </>
  );
}
