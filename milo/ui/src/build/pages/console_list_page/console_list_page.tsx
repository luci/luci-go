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
import { Fragment, useEffect } from 'react';
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { useInfinitePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { MiloInternal } from '@/common/services/milo_internal';

import { ConsoleSnapshotRow } from './console_snapshot_row';
import { ProjectIdBar } from './project_id_bar';

export function ConsoleListPage() {
  const { project } = useParams();
  if (!project) {
    throw new Error('invariant violated: project should be set');
  }

  const { data, error, isError, isLoading, fetchNextPage, hasNextPage } =
    useInfinitePrpcQuery({
      host: '',
      insecure: location.protocol === 'http:',
      Service: MiloInternal,
      method: 'queryConsoleSnapshots',
      request: { predicate: { project }, pageSize: 100 },
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
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="console-list">
      <ConsoleListPage />
    </RecoverableErrorBoundary>
  );
}
