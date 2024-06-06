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
import { useParams } from 'react-router-dom';

import {
  getPageSize,
  getPageToken,
} from '@/build/pages/builder_page/ended_builds_section/search_param_utils';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { ParamsPager } from '@/common/components/params_pager';
import { UiPage } from '@/common/constants/view';
import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  ListStatusRequest,
  TreeStatusClientImpl,
} from '@/proto/go.chromium.org/luci/tree_status/proto/v1/tree_status.pb';
import { TreeStatusTable } from '@/tree_status/components/tree_status_table';
import { TreeStatusUpdater } from '@/tree_status/components/tree_status_updater';

export const TreeStatusListPage = () => {
  const { tree: treeName } = useParams();
  const [searchParams, _] = useSyncedSearchParams();
  const pageSize = getPageSize(searchParams);
  const pageToken = getPageToken(searchParams);

  const treeStatusClient = usePrpcServiceClient({
    host: SETTINGS.luciTreeStatus.host,
    ClientImpl: TreeStatusClientImpl,
  });
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
  if (!treeName) {
    return <Alert severity="error">No tree name specified</Alert>;
  }
  if (status.isError) {
    throw status.error;
  }
  return (
    <div>
      <PageMeta
        title="Tree Status"
        selectedPage={UiPage.TreeStatus}
        project={treeName}
      />
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
      {status.isLoading ? (
        <CircularProgress />
      ) : status.data.status.length === 0 ? (
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
        <TreeStatusTable status={status.data.status} />
      )}
      <ParamsPager nextPageToken={status.data?.nextPageToken || ''} />
    </div>
  );
};

export function Component() {
  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="tree-status-page">
      <TreeStatusListPage />
    </RecoverableErrorBoundary>
  );
}
