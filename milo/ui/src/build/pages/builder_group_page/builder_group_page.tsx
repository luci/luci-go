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

import { LinearProgress } from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useEffect, useMemo } from 'react';
import { Helmet } from 'react-helmet';
import { useParams } from 'react-router';

import { FilterableBuilderTable } from '@/build/components/filterable_builder_table';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  useDeclarePageId,
  useEstablishProjectCtx,
} from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { useMiloInternalClient } from '@/common/hooks/prpc_clients';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { ListBuildersRequest } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

import { BuilderGroupIdBar } from './builder_group_id_bar';

export function BuilderGroupPage() {
  const { project, group } = useParams();
  if (!project || !group) {
    throw new Error('invariant violated: project, group must be set');
  }
  useEstablishProjectCtx(project);

  const client = useMiloInternalClient();
  const { data, isPending, error, isError, fetchNextPage, hasNextPage } =
    useInfiniteQuery({
      ...client.ListBuilders.queryPaged(
        ListBuildersRequest.fromPartial({
          project,
          group,
        }),
      ),
    });

  if (isError) {
    throw error;
  }

  // Keep loading until all pages have been loaded.
  useEffect(() => {
    if (isPending || !hasNextPage) {
      return;
    }
    fetchNextPage();
  }, [isPending, hasNextPage, data?.pages.length, fetchNextPage]);

  const builders = useMemo(
    () => data?.pages.flatMap((p) => p.builders.map((b) => b.id!)) || [],
    [data],
  );

  return (
    <>
      <Helmet>
        <title>
          {project} | {group} | Builders
        </title>
      </Helmet>
      <BuilderGroupIdBar project={project} group={group} />
      <LinearProgress
        value={100}
        variant={isPending ? 'indeterminate' : 'determinate'}
        color="primary"
      />
      <FilterableBuilderTable
        builders={builders}
        // Each builder table row needs 3 SearchBuilds RPC. So the number should
        // be a multiple of 3 to achieve best results.
        // 9 is picked to achieve a balance between HTTP/server overhead and
        // RPC latency (< 1s). This can be adjust upwards once the SearchBuilds
        // RPC is optimized to support the builder table.
        maxBatchSize={9}
      />
    </>
  );
}

export function Component() {
  useDeclarePageId(UiPage.Builders);

  return (
    <TrackLeafRoutePageView contentGroup="builder-group">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="builder-group"
      >
        <BuilderGroupPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
