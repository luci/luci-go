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

import { Input, LinearProgress } from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useEffect, useMemo } from 'react';
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { SearchInput } from '@/common/components/search_input';
import { UiPage } from '@/common/constants/view';
import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  BuildersClientImpl,
  ListBuildersRequest,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_service.pb';

import { BuilderGroupIdBar } from './builder_group_id_bar';
import { BuilderTable } from './builder_table';

const DEFAULT_NUM_OF_BUILDS = 25;

function getNumOfBuilds(params: URLSearchParams) {
  return Number(params.get('numOfBuilds')) || DEFAULT_NUM_OF_BUILDS;
}

function numOfBuildsUpdater(newNumOfBuilds: number) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (newNumOfBuilds === DEFAULT_NUM_OF_BUILDS) {
      searchParams.delete('numOfBuilds');
    } else {
      searchParams.set('numOfBuilds', String(newNumOfBuilds));
    }
    return searchParams;
  };
}

function getFilter(params: URLSearchParams) {
  return params.get('q') || '';
}

function filterUpdater(newSearchQuery: string) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (newSearchQuery === '') {
      searchParams.delete('q');
    } else {
      searchParams.set('q', newSearchQuery);
    }
    return searchParams;
  };
}

export function BuilderListPage() {
  const { project } = useParams();
  if (!project) {
    throw new Error('invariant violated: project should be set');
  }

  const [searchParams, setSearchparams] = useSyncedSearchParams();
  const numOfBuilds = getNumOfBuilds(searchParams);
  const filter = getFilter(searchParams);

  const client = usePrpcServiceClient({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildersClientImpl,
  });
  const { data, isLoading, error, isError, fetchNextPage, hasNextPage } =
    useInfiniteQuery(
      client.ListBuilders.queryPaged(
        ListBuildersRequest.fromPartial({
          project,
        }),
      ),
    );

  if (isError) {
    throw error;
  }

  // Keep loading until all pages have been loaded.
  useEffect(() => {
    if (isLoading || !hasNextPage) {
      return;
    }
    fetchNextPage();
  }, [isLoading, hasNextPage, data?.pages.length, fetchNextPage]);

  const builders = useMemo(
    () =>
      data?.pages
        .flatMap((p) => p.builders.map((b) => b.id!))
        .map((bid) => {
          return [
            // Pre-compute to support case-insensitive searching.
            `${bid.project}/${bid.bucket}/${bid.builder}`.toLowerCase(),
            bid,
          ] as const;
        }) || [],
    [data],
  );

  const filteredBuilders = useMemo(() => {
    const parts = filter.toLowerCase().split(' ');
    return builders
      .filter(([lowerBuilderId]) =>
        parts.every((part) => lowerBuilderId.includes(part)),
      )
      .map(([_, builder]) => builder);
  }, [filter, builders]);

  return (
    <>
      <PageMeta
        project={project}
        selectedPage={UiPage.Builders}
        title={`${project} | Builders`}
      />
      <BuilderGroupIdBar project={project} />
      <LinearProgress
        value={100}
        variant={isLoading ? 'indeterminate' : 'determinate'}
        color="primary"
      />
      <div css={{ margin: '10px' }}>
        <SearchInput
          placeholder="Press '/' to search builders."
          value={filter}
          focusShortcut="/"
          onValueChange={(v) => setSearchparams(filterUpdater(v))}
          initDelayMs={500}
        />
      </div>
      <BuilderTable builders={filteredBuilders} numOfBuilds={numOfBuilds} />
      <div css={{ margin: '5px 5px' }}>
        <div>Showing {builders.length} builders.</div>
        <div>
          Number of builds per builder (10-100):
          <Input
            type="number"
            value={numOfBuilds}
            inputProps={{
              min: 10,
              max: 100,
            }}
            onChange={(e) =>
              setSearchparams(numOfBuildsUpdater(Number(e.target.value)))
            }
          />
        </div>
      </div>
    </>
  );
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="builders">
    <BuilderListPage />
  </RecoverableErrorBoundary>
);
