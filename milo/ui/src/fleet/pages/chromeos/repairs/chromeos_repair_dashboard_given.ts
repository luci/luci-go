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

import { RepairIrmIncident } from '@/fleet/components/repair_queue/active_irm_table';
import { enableChromeOsWorkforceActivity } from '@/fleet/features';
import {
  REPAIR_LAB_LOCAL_STORAGE_KEY,
  RepairLab,
} from '@/fleet/pages/chromeos/repairs/use_repair_lab';
import {
  DEFAULT_REPAIR_QUEUE_DIMENSIONS,
  RepairQueueItemBuilder,
} from '@/fleet/testing_tools/builders/repair_queue_item_builder';
import { genericGiven } from '@/fleet/testing_tools/integration/fleet_integration_test';
import { fakeFleetConsoleBackend } from '@/fleet/testing_tools/mock_api/fake_fleet_console_backend';
import {
  IrmIncident,
  PriorityRule,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

const setupRepairDashboardTest = () => {
  beforeEach(() => {
    fakeFleetConsoleBackend.setStaticResponse(
      'GetDeviceDimensions',
      DEFAULT_REPAIR_QUEUE_DIMENSIONS,
    );
    fakeFleetConsoleBackend.priorityRules.seed([]);
    fakeFleetConsoleBackend.irm.seed([]);
  });
};

setupRepairDashboardTest();

export const given = {
  ...genericGiven,

  userCanEditPriorityRules(canEdit: boolean) {
    genericGiven.permission(canEdit);
  },

  repairQueue(builders: RepairQueueItemBuilder[]) {
    const items = builders.map((b) => b.build());
    fakeFleetConsoleBackend.repairQueue.seed({
      repairQueueItems: items,
      totalSize: items.length,
      nextPageToken: '',
    });
  },

  emptyRepairQueue() {
    fakeFleetConsoleBackend.repairQueue.seed({
      repairQueueItems: [],
      totalSize: 0,
      nextPageToken: '',
    });
  },

  repairQueueError(grpcCode: number, message: string) {
    genericGiven.rpcError('ListRepairQueue', grpcCode, message);
  },

  claimError(grpcCode: number, message: string) {
    genericGiven.rpcError('ClaimRepairTask', grpcCode, message);
  },

  priorityRules(rules: PriorityRule[]) {
    fakeFleetConsoleBackend.priorityRules.seed(rules);
  },

  priorityRulesError(grpcCode: number, message: string) {
    genericGiven.rpcError('ListPriorityRules', grpcCode, message);
  },

  storedRepairLab(lab: RepairLab) {
    window.localStorage.setItem(REPAIR_LAB_LOCAL_STORAGE_KEY, lab);
  },

  irmIncidents(incidents: readonly RepairIrmIncident[]) {
    fakeFleetConsoleBackend.irm.seed(
      incidents.map((i) => IrmIncident.create(i)),
    );
  },

  emptyIrmIncidents() {
    fakeFleetConsoleBackend.irm.seed([]);
  },

  irmIncidentsError(grpcCode: number, message: string) {
    genericGiven.rpcError('ListIrmIncidents', grpcCode, message);
  },

  workforceActivityEnabled() {
    genericGiven.featureFlag(enableChromeOsWorkforceActivity, true);
  },

  workforceActivityDisabled() {
    genericGiven.featureFlag(enableChromeOsWorkforceActivity, false);
  },
};
