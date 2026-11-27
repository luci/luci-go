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

import { useState } from 'react';
import { useNavigate } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFeatureFlag } from '@/common/feature_flags';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { SharedWorkforceActivityView } from '@/fleet/components/repair_queue/workforce_activity_view';
import { TimeframeOption } from '@/fleet/components/repair_queue/workforce_helpers';
import { generateRepairsURL } from '@/fleet/constants/paths';
import {
  enableAndroidRepairQueue,
  enableAndroidWorkforceActivity,
} from '@/fleet/features';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { PageNotFoundPage } from '@/fleet/pages/not_found_page';
import { AndroidPageWorkspace, workspaces } from '@/fleet/workspaces';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { UnclaimAndroidRepairTaskRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { useAndroidWorkforceActivity } from './use_android_workforce_activity';
import { useUnclaimAndroidRepairTask } from './use_claim_android_repair_task';

const ANDROID_WORKFORCE_LABELS = {
  inProgressSubtitle: 'Currently claimed devices',
  claimedColumnHeader: 'Currently Claimed Devices (In-Progress)',
  scoreUnit: 'device',
} as const;

export interface AndroidWorkforceActivityViewProps {
  workspace: AndroidPageWorkspace;
  onBackToQueue?: () => void;
}

export const AndroidWorkforceActivityView = ({
  workspace,
  onBackToQueue,
}: AndroidWorkforceActivityViewProps) => {
  const navigate = useNavigate();
  const [timeframe, setTimeframe] = useState<TimeframeOption>('1D');
  const { data, isPending, isError, error } = useAndroidWorkforceActivity(
    workspace,
    timeframe,
  );
  const { mutateAsync: unclaimTask, isPending: isUnclaimPending } =
    useUnclaimAndroidRepairTask();

  return (
    <SharedWorkforceActivityView
      data={data}
      isPending={isPending}
      isError={isError}
      error={error}
      timeframe={timeframe}
      onTimeframeChange={setTimeframe}
      onBackToQueue={
        onBackToQueue ??
        (() => navigate(generateRepairsURL(workspaces[workspace].baseUrl)))
      }
      onUnclaimTask={(taskId) =>
        unclaimTask(UnclaimAndroidRepairTaskRequest.fromPartial({ taskId }))
      }
      isUnclaimPending={isUnclaimPending}
      labels={ANDROID_WORKFORCE_LABELS}
    />
  );
};

function AndroidWorkforceActivityPageContent({
  workspace,
}: {
  workspace: AndroidPageWorkspace;
}) {
  const isRepairQueueEnabled = useFeatureFlag(enableAndroidRepairQueue);
  const isWorkforceEnabled = useFeatureFlag(enableAndroidWorkforceActivity);
  if (!isRepairQueueEnabled || !isWorkforceEnabled) {
    return <PageNotFoundPage />;
  }
  return (
    <div css={{ margin: '24px', paddingBottom: '40px' }}>
      <AndroidWorkforceActivityView workspace={workspace} />
    </div>
  );
}

export function Component({ workspace }: { workspace: AndroidPageWorkspace }) {
  const slug = workspace.toLowerCase();
  return (
    <TrackLeafRoutePageView
      contentGroup={`fleet-console-${slug}-repairs-workforce`}
    >
      <FleetHelmet pageTitle={`${workspace} Workforce Activity`} />
      <RecoverableErrorBoundary key={`fleet-${slug}-repairs-workforce`}>
        <LoggedInBoundary>
          <AndroidWorkforceActivityPageContent workspace={workspace} />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}

export default Component;
