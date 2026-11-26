// Copyright 2026 The LUCI Authors.
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

import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { TimeframeOption } from '@/fleet/components/repair_queue/workforce_helpers';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import {
  GetAndroidWorkforceActivityRequest,
  GetAndroidWorkforceActivityRequest_Timeframe,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { toAndroidWorkspaceEnum } from './android_workspace';

export const ANDROID_WORKFORCE_ACTIVITY_QUERY_KEY = [
  'androidWorkforceActivity',
] as const;

const ANDROID_TIMEFRAME_TO_PROTO: Record<
  TimeframeOption,
  GetAndroidWorkforceActivityRequest_Timeframe
> = {
  '1D': GetAndroidWorkforceActivityRequest_Timeframe.TIMEFRAME_1D,
  '7D': GetAndroidWorkforceActivityRequest_Timeframe.TIMEFRAME_7D,
  '30D': GetAndroidWorkforceActivityRequest_Timeframe.TIMEFRAME_30D,
  YTD: GetAndroidWorkforceActivityRequest_Timeframe.TIMEFRAME_YTD,
};

export const useAndroidWorkforceActivity = (
  workspace: AndroidPageWorkspace,
  timeframe: TimeframeOption,
) => {
  const client = useFleetConsoleClient();
  const request = useMemo(
    () =>
      GetAndroidWorkforceActivityRequest.fromPartial({
        workspace: toAndroidWorkspaceEnum(workspace),
        timeframe: ANDROID_TIMEFRAME_TO_PROTO[timeframe],
      }),
    [workspace, timeframe],
  );
  return useQuery({
    ...client.GetAndroidWorkforceActivity.query(request),
    queryKey: [...ANDROID_WORKFORCE_ACTIVITY_QUERY_KEY, request],
    refetchInterval: 10000,
  });
};
