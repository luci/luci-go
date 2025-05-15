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

import { Alert, CircularProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { Helmet } from 'react-helmet';
import { useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  useDeclarePageId,
  useEstablishProjectCtx,
} from '@/common/components/page_meta';
import {
  ParamsPager,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { UiPage } from '@/common/constants/view';
import { useTreesClient } from '@/common/hooks/prpc_clients';
import { logging } from '@/common/tools/logging';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ListStatusRequest } from '@/proto/go.chromium.org/luci/tree_status/proto/v1/tree_status.pb';
import { GetTreeRequest } from '@/proto/go.chromium.org/luci/tree_status/proto/v1/trees.pb';
import { TreeStatusTable } from '@/tree_status/components/tree_status_table';
import { TreeStatusUpdater } from '@/tree_status/components/tree_status_updater';
import { useTreeStatusClient } from '@/tree_status/hooks/prpc_clients';

export const TreeStatusListPage = () => {
  const { tree: treeName } = useParams();
  if (!treeName) {
    throw new Error('invariant violated: tree must be set');
  }

  const pagerCtx = usePagerContext({
    pageSizeOptions: [25, 50, 100, 200],
    defaultPageSize: 50,
  });
  const [searchParams] = useSyncedSearchParams();
  const pageSize = getPageSize(pagerCtx, searchParams);
  const pageToken = getPageToken(pagerCtx, searchParams);

  const treeStatusClient = useTreeStatusClient();
  const status = useQuery({
    // eslint-disable-next-line new-cap
    ...treeStatusClient.ListStatus.query(
      ListStatusRequest.fromPartial({
        parent: `trees/${treeName}/status`,
        pageSize,
        pageToken,
      }),
    ),
    refetchInterval: 60000,
    enabled: !!treeName,
  });

  // Get tree.
  const treesClient = useTreesClient();
  const tree = useQuery({
    // eslint-disable-next-line new-cap
    ...treesClient.GetTree.query(
      GetTreeRequest.fromPartial({
        name: `trees/${treeName}`,
      }),
    ),
    // We only need to query if project is not specified.
    enabled: !!treeName && !searchParams.get('project'),
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  });

  if (status.isError) {
    throw status.error;
  }
  if (tree.error) {
    // Just log, this is only for the project name, it is not crucial for the page.
    logging.error('failed to get tree', tree.error);
  }
  // project will be set based on the following:
  // - If there is project field in search params, it will be set based on search params.
  // - If not, it will be set based on the result from GetTree RPC.
  // - If GetTree RPC failed, it will default to fall back to tree name.
  const project =
    searchParams.get('project') ||
    (tree.data && tree.data.projects.length > 0
      ? tree.data.projects[0]
      : treeName);
  useEstablishProjectCtx(project);

  return (
    <div>
      <Alert severity="info">
        <strong>Tree status for the tree {treeName}</strong>
        <p>
          The tree status signals whether the source tree is <em>open</em>{' '}
          (accepting CL merges) or <em>closed</em> (not currently accepting CL
          merges).
        </p>
        <p>
          Tree status does not do anything by itself, but is interpreted by
          other tools when performing actions that affect the tree. Because of
          this, the states <em>throttled</em> and <em>maintenance</em> have
          varying meaning and should only be used if you know what the effect on
          your tooling will be.
        </p>
        <strong>
          Note: The tree status should be updated by the current on-call user.
        </strong>
      </Alert>
      <div style={{ marginTop: '40px', height: '0px' }} />
      <TreeStatusUpdater tree={treeName} />
      <div style={{ marginTop: '40px', height: '0px' }} />
      {status.isPending ? (
        <CircularProgress />
      ) : status?.data?.status.length === 0 ? (
        <Alert severity="warning">
          <strong>
            There are no status updates currently recorded for the {treeName}{' '}
            tree.
          </strong>
          <br />
          This could either be a new tree or it may be more than 140 days since
          the last status update. Most systems will treat this as an open tree,
          but it is better to explicitly add an open status.
        </Alert>
      ) : (
        <TreeStatusTable status={status?.data?.status || []} />
      )}
      <ParamsPager
        pagerCtx={pagerCtx}
        nextPageToken={status.data?.nextPageToken || ''}
      />
    </div>
  );
};

export function Component() {
  useDeclarePageId(UiPage.TreeStatus);

  return (
    <TrackLeafRoutePageView contentGroup="tree-status-list">
      <Helmet>
        <title>Tree status</title>
      </Helmet>
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="tree-status-list"
      >
        <TreeStatusListPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
