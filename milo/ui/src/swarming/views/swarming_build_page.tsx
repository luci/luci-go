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
import { useEffect, useMemo } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { usePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { TasksServices } from '@/common/services/swarming';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { getBuildURLPathFromTags } from '@/swarming/tools/utils';

const ALLOWED_HOSTS = Object.freeze([
  SETTINGS.swarming.defaultHost,
  ...(SETTINGS.swarming.allowedHosts || []),
]);

export function SwarmingBuildPage() {
  const { taskId } = useParams();
  const [searchParams] = useSyncedSearchParams();
  const navigate = useNavigate();

  if (!taskId) {
    throw new Error('invariant violated: taskId should be set');
  }

  const swarmingHost =
    searchParams.get('server') || SETTINGS.swarming.defaultHost;
  if (!ALLOWED_HOSTS.includes(swarmingHost)) {
    throw new Error(`'${swarmingHost}' is not an allowed host`);
  }

  const { data, error } = usePrpcQuery({
    host: swarmingHost,
    Service: TasksServices,
    method: 'getRequest',
    request: {
      taskId,
    },
  });

  if (error) {
    throw error;
  }

  const targetUrlPath = useMemo(
    () => data?.tags && getBuildURLPathFromTags(data.tags),
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

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="swarming-build">
    <SwarmingBuildPage />
  </RecoverableErrorBoundary>
);
