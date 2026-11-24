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

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { SharedWorkforceActivityView } from '@/fleet/components/repair_queue/workforce_activity_view';
import {
  TimeframeOption,
  WorkforceActivityData,
} from '@/fleet/components/repair_queue/workforce_helpers';
import {
  CHROMEOS_PLATFORM,
  generateRepairsURL,
  platformToURL,
} from '@/fleet/constants/paths';
import { useCurrentPlatform } from '@/fleet/hooks/usePlatform';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import {
  GetWorkforceActivityRequest,
  GetWorkforceActivityRequest_Timeframe,
  GetWorkforceActivityResponse,
  UnclaimRepairTaskRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { LabSelector } from './lab_selector';
import { useUnclaimRepairTask } from './use_claim_repair_task';
import { RepairLab, useRepairLab } from './use_repair_lab';
import { useWorkforceActivity } from './use_workforce_activity';

const CHROMEOS_TIMEFRAME_TO_PROTO: Record<
  TimeframeOption,
  GetWorkforceActivityRequest_Timeframe
> = {
  '1D': GetWorkforceActivityRequest_Timeframe.TIMEFRAME_1D,
  '7D': GetWorkforceActivityRequest_Timeframe.TIMEFRAME_7D,
  '30D': GetWorkforceActivityRequest_Timeframe.TIMEFRAME_30D,
  YTD: GetWorkforceActivityRequest_Timeframe.TIMEFRAME_YTD,
};

export interface WorkforceActivityViewProps {
  onBackToQueue?: () => void;
  lab?: RepairLab;
}

export const WorkforceActivityView = ({
  onBackToQueue,
  lab: labProp,
}: WorkforceActivityViewProps) => {
  const navigate = useNavigate();
  const currentPlatform = useCurrentPlatform();
  const { lab: contextLab } = useRepairLab();
  const lab = labProp ?? contextLab;
  const [timeframe, setTimeframe] = useState<TimeframeOption>('1D');

  const request = useMemo(
    () =>
      GetWorkforceActivityRequest.fromPartial({
        lab,
        timeframe: CHROMEOS_TIMEFRAME_TO_PROTO[timeframe],
      }),
    [lab, timeframe],
  );
  const { data, isPending, isError, error } = useWorkforceActivity(request);
  const viewData = useMemo(
    () => (data ? toWorkforceActivityData(data) : undefined),
    [data],
  );
  const { mutateAsync: unclaimTask, isPending: isUnclaimPending } =
    useUnclaimRepairTask();

  return (
    <SharedWorkforceActivityView
      data={viewData}
      isPending={isPending}
      isError={isError}
      error={error}
      timeframe={timeframe}
      onTimeframeChange={setTimeframe}
      onBackToQueue={
        onBackToQueue ??
        (() =>
          navigate(
            generateRepairsURL(
              currentPlatform
                ? platformToURL(currentPlatform)
                : CHROMEOS_PLATFORM,
            ) + `?lab=${lab}`,
          ))
      }
      onUnclaimTask={(taskId) =>
        unclaimTask(UnclaimRepairTaskRequest.fromPartial({ taskId }))
      }
      isUnclaimPending={isUnclaimPending}
      headerActions={<LabSelector value={lab} />}
    />
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-chromeos-repairs-workforce">
      <FleetHelmet pageTitle="ChromeOS Workforce Activity" />
      <RecoverableErrorBoundary key="fleet-chromeos-repairs-workforce">
        <LoggedInBoundary>
          <div
            css={{
              margin: '24px',
              paddingBottom: '40px',
            }}
          >
            <WorkforceActivityView />
          </div>
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}

export default Component;

function toWorkforceActivityData(
  resp: GetWorkforceActivityResponse,
): WorkforceActivityData {
  return {
    activeRepairers: resp.activeRepairers,
    totalPriorityPointsCleared: resp.totalPriorityPointsCleared,
    avgQueuePickupRank: resp.avgQueuePickupRank,
    inProgressRepairs: resp.inProgressRepairs,
    avgRepairTime: resp.avgRepairTime,
    technicians: resp.technicians.map((tech) => ({
      id: tech.id,
      name: tech.name,
      email: tech.email,
      claimedDevices: tech.claimedDuts.map((dut) => ({
        deviceId: dut.dutId,
        score: dut.score,
        duration: dut.duration,
        taskId: dut.taskId,
      })),
      priorityScoreCleared: tech.priorityScoreCleared,
      avgPtsPerDevice: tech.avgPtsPerDut,
      avgQueuePickupRank: tech.avgQueuePickupRank,
      completedRepairs: tech.completedRepairs,
      avgDuration: tech.avgDuration,
    })),
  };
}
