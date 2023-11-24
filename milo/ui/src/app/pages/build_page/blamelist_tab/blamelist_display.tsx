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

import { useInfinitePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { BuilderID } from '@/common/services/buildbucket';
import { GitilesCommit } from '@/common/services/common';
import { MiloInternal } from '@/common/services/milo_internal';
import { getGitilesRepoURL } from '@/common/tools/gitiles_utils';

import { BlamelistTable } from './blamelist_table';
import { RevisionRange } from './revision_range';

export interface BlamelistDisplayProps {
  readonly blamelistPin: GitilesCommit;
  readonly builder: BuilderID;
}

export function BlamelistDisplay({
  blamelistPin,
  builder,
}: BlamelistDisplayProps) {
  const {
    data,
    error,
    isError,
    hasNextPage,
    fetchNextPage,
    isLoading,
    isFetchingNextPage,
  } = useInfinitePrpcQuery({
    host: '',
    insecure: location.protocol === 'http:',
    Service: MiloInternal,
    method: 'queryBlamelist',
    request: {
      gitilesCommit: blamelistPin,
      builder,
    },
    options: {
      // The query is expensive and the blamelist should be stable anyway.
      refetchOnWindowFocus: false,
    },
  });

  if (isError) {
    throw error;
  }

  const isLoadingPage = isLoading || isFetchingNextPage;
  const commitCount =
    data?.pages.reduce((prev, page) => prev + (page.commits?.length || 0), 0) ||
    0;

  return (
    <>
      <div css={{ padding: '5px 10px' }}>
        <RevisionRange
          blamelistPin={blamelistPin}
          commitCount={commitCount}
          precedingCommit={
            hasNextPage
              ? null
              : data?.pages[data.pages.length - 1].precedingCommit
          }
        />
      </div>
      <BlamelistTable
        repoUrl={getGitilesRepoURL(blamelistPin)}
        pages={data?.pages || []}
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
