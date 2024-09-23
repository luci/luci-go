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

import { LinearProgress } from '@mui/material';
import { InfiniteData, useInfiniteQuery } from '@tanstack/react-query';
import { Fragment, useEffect } from 'react';
import { useParams } from 'react-router-dom';

import { OutputQueryConsoleSnapshotsResponse } from '@/build/types';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { useMiloInternalClient } from '@/common/hooks/prpc_clients';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { QueryConsoleSnapshotsRequest } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

import { ConsoleSnapshotRow } from './console_snapshot_row';
import { ProjectIdBar } from './project_id_bar';

export function ConsoleListPage() {
  const { project } = useParams();
  if (!project) {
    throw new Error('invariant violated: project should be set');
  }

  const client = useMiloInternalClient();
  const { data, error, isError, isLoading, fetchNextPage, hasNextPage } =
    useInfiniteQuery({
      ...client.QueryConsoleSnapshots.queryPaged(
        QueryConsoleSnapshotsRequest.fromPartial({
          predicate: { project },
          pageSize: 100,
        }),
      ),
      select: (data) =>
        data as InfiniteData<OutputQueryConsoleSnapshotsResponse>,
    });

  if (isError) {
    throw error;
  }

  useEffect(() => {
    if (!isLoading && hasNextPage) {
      fetchNextPage();
    }
  }, [isLoading, fetchNextPage, hasNextPage, data?.pages.length]);

  return (
    <>
      <PageMeta
        project={project}
        selectedPage={UiPage.BuilderGroups}
        title={`${project} | Builder Groups`}
      />
      <ProjectIdBar project={project} />
      <LinearProgress
        value={100}
        variant={isLoading ? 'indeterminate' : 'determinate'}
        color="primary"
      />
      <table css={{ width: '100%' }}>
        <tbody>
          {data?.pages.map((page, i) => (
            <Fragment key={i}>
              {page.snapshots?.map((snapshot) => (
                <ConsoleSnapshotRow
                  key={snapshot.console.id}
                  snapshot={snapshot}
                />
              ))}
            </Fragment>
          ))}
        </tbody>
      </table>
    </>
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="console-list">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="console-list"
      >
        <ConsoleListPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
