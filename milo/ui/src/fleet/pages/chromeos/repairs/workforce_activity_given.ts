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
  enableChromeOsWorkforceActivity,
  enableWorkforceInProgressRepairs,
  enableWorkforcePickupRank,
  enableWorkforceTimeframeFilter,
} from '@/fleet/features';
import {
  REPAIR_LAB_LOCAL_STORAGE_KEY,
  RepairLab,
} from '@/fleet/pages/chromeos/repairs/use_repair_lab';
import { genericGiven } from '@/fleet/testing_tools/integration/fleet_integration_test';
import {
  GetWorkforceActivityResponse,
  TechnicianActivity,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export type WorkforceActivityInput = Parameters<
  typeof GetWorkforceActivityResponse.fromPartial
>[0];

export type TechnicianActivityInput = Parameters<
  typeof TechnicianActivity.fromPartial
>[0];

export const buildWorkforceActivityJson = (
  data: WorkforceActivityInput,
): unknown =>
  GetWorkforceActivityResponse.toJSON(
    GetWorkforceActivityResponse.fromPartial(data),
  );

export const aTechnicianActivity = (
  overrides: TechnicianActivityInput = {},
): TechnicianActivityInput => ({
  id: 'tech-1',
  name: 'Andrew Miller',
  email: 'andrew@google.com',
  claimedDuts: [],
  priorityScoreCleared: 0,
  avgPtsPerDut: 0,
  avgQueuePickupRank: 0,
  isRankSkewed: false,
  completedRepairs: 0,
  avgDuration: undefined,
  ...overrides,
});

export const given = {
  ...genericGiven,

  workforceActivity(data: WorkforceActivityInput) {
    genericGiven.rpcResponse(
      'GetWorkforceActivity',
      buildWorkforceActivityJson(data),
    );
  },

  emptyWorkforceActivity() {
    genericGiven.rpcResponse(
      'GetWorkforceActivity',
      buildWorkforceActivityJson({ technicians: [] }),
    );
  },

  workforceActivityError(grpcCode: number, message: string) {
    genericGiven.rpcError('GetWorkforceActivity', grpcCode, message);
  },

  deferredWorkforceActivity() {
    return genericGiven.deferredRpc('GetWorkforceActivity');
  },

  deferredUnclaim() {
    return genericGiven.deferredRpc('UnclaimRepairTask');
  },

  allMetricFlagsEnabled() {
    genericGiven.featureFlag(enableWorkforcePickupRank, true);
    genericGiven.featureFlag(enableWorkforceInProgressRepairs, true);
    genericGiven.featureFlag(enableWorkforceTimeframeFilter, true);
  },

  pickupRankEnabled(enabled = true) {
    genericGiven.featureFlag(enableWorkforcePickupRank, enabled);
  },

  inProgressRepairsEnabled(enabled = true) {
    genericGiven.featureFlag(enableWorkforceInProgressRepairs, enabled);
  },

  timeframeFilterEnabled(enabled = true) {
    genericGiven.featureFlag(enableWorkforceTimeframeFilter, enabled);
  },

  workforceActivityEnabled(enabled = true) {
    genericGiven.featureFlag(enableChromeOsWorkforceActivity, enabled);
  },

  workforceActivityDisabled() {
    genericGiven.featureFlag(enableChromeOsWorkforceActivity, false);
  },

  storedRepairLab(lab: RepairLab) {
    window.localStorage.setItem(REPAIR_LAB_LOCAL_STORAGE_KEY, lab);
  },
};
