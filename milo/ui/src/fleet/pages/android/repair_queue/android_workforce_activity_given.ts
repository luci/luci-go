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

import {
  enableAndroidRepairQueue,
  enableWorkforceInProgressRepairs,
  enableWorkforcePickupRank,
  enableWorkforceTimeframeFilter,
} from '@/fleet/features';
import { genericGiven } from '@/fleet/testing_tools/integration/fleet_integration_test';
import {
  AndroidTechnicianActivity,
  GetAndroidWorkforceActivityResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

type AndroidWorkforceActivityInput = Parameters<
  typeof GetAndroidWorkforceActivityResponse.fromPartial
>[0];

type AndroidTechnicianActivityInput = Parameters<
  typeof AndroidTechnicianActivity.fromPartial
>[0];

export const anAndroidTechnician = (
  overrides: AndroidTechnicianActivityInput = {},
): AndroidTechnicianActivityInput => ({
  id: 'tech-1',
  name: 'Alex Rivera',
  email: 'alex@example.com',
  claimedDevices: [],
  priorityScoreCleared: 0,
  avgPtsPerDevice: 0,
  avgQueuePickupRank: 0,
  completedRepairs: 0,
  avgDuration: undefined,
  ...overrides,
});

export const given = {
  ...genericGiven,

  workforceActivity(data: AndroidWorkforceActivityInput) {
    genericGiven.rpcResponse(
      'GetAndroidWorkforceActivity',
      GetAndroidWorkforceActivityResponse.toJSON(
        GetAndroidWorkforceActivityResponse.fromPartial(data),
      ),
    );
  },

  workforceActivityError(grpcCode: number, message: string) {
    genericGiven.rpcError('GetAndroidWorkforceActivity', grpcCode, message);
  },

  deferredUnclaimAndroidTask() {
    return genericGiven.deferredRpc('UnclaimAndroidRepairTask');
  },

  unclaimAndroidTaskError(grpcCode: number, message: string) {
    genericGiven.rpcError('UnclaimAndroidRepairTask', grpcCode, message);
  },

  allFlagsEnabled() {
    genericGiven.featureFlag(enableAndroidRepairQueue, true);
    genericGiven.featureFlag(enableWorkforcePickupRank, true);
    genericGiven.featureFlag(enableWorkforceInProgressRepairs, true);
    genericGiven.featureFlag(enableWorkforceTimeframeFilter, true);
  },

  androidRepairQueueEnabled(enabled = true) {
    genericGiven.featureFlag(enableAndroidRepairQueue, enabled);
  },
};
