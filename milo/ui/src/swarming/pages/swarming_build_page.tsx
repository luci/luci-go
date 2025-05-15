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

import { CircularProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useMemo } from 'react';
import { useNavigate, useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { TaskIdRequest } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { getBuildURLPathFromTags } from '@/swarming/tools/utils';

import { useTasksClient } from '../hooks/prpc_clients';

export function SwarmingBuildPage() {
  const { taskId } = useParams();
  const [searchParams] = useSyncedSearchParams();
  const navigate = useNavigate();

  if (!taskId) {
    throw new Error('invariant violated: taskId must be set');
  }

  const swarmingHost =
    searchParams.get('server') || SETTINGS.swarming.defaultHost;

  const client = useTasksClient(swarmingHost);

  const { data, error } = useQuery(
    client.GetRequest.query(
      TaskIdRequest.fromPartial({
        taskId,
      }),
    ),
  );

  if (error) {
    throw error;
  }

  const targetUrlPath = useMemo(
    () => data && getBuildURLPathFromTags(data.tags),
    [data],
  );

  useEffect(() => {
    if (!targetUrlPath) {
      return;
    }

    navigate(targetUrlPath);
  }, [targetUrlPath, navigate]);

  if (data && !targetUrlPath) {
    throw new Error(`task: ${taskId} does not have an associated build.`);
  }

  return (
    <div css={{ padding: '10px' }}>
      Redirecting to the build page associated with the task: {taskId}{' '}
      <CircularProgress />
    </div>
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="swarming-build">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="swarming-build"
      >
        <SwarmingBuildPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
