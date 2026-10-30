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
  PeripheralState,
  PriorityRule,
  RepairQueueItem,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const DEFAULT_REPAIR_QUEUE_DIMENSIONS = {
  baseDimensions: {
    id: {
      values: ['chromeos15-row2-rack3-host4', 'chromeos15-row2-rack3-host5'],
    },
    dut_state: { values: ['needs_repair', 'repair_failed'] },
    'label-board': { values: ['volteer', 'brya'] },
    'label-model': { values: ['volteer', 'brya', 'corsola'] },
    'label-pool': { values: ['DUT_POOL_QUOTA', 'faft-cr50'] },
  },
  labels: {
    'label-lab': { values: ['MTV', 'SFO'] },
    'label-irm': { values: ['b/458449804', 'b/542600013'] },
  },
};

type Mutable<T> = {
  -readonly [P in keyof T]: T[P];
};

/**
 * Fluent test data builder for `RepairQueueItem` so each integration test can
 * specify only the minimal fields relevant to its scenario.
 */
export class RepairQueueItemBuilder {
  private item: Mutable<RepairQueueItem>;

  constructor(index = 1) {
    const dutId = `chromeos15-row2-rack3-host${index}`;
    this.item = RepairQueueItem.fromPartial({
      taskId: `task-${index}`,
      dutId,
      deviceId: dutId,
      pools: ['DUT_POOL_QUOTA'],
      model: 'volteer',
      state: 'needs_repair',
      claimedBy: '',
      claimedAt: undefined,
      servoState: PeripheralState.PERIPHERAL_STATE_OK,
      wifiState: PeripheralState.PERIPHERAL_STATE_BROKEN,
      bluetoothState: PeripheralState.PERIPHERAL_STATE_MISSING,
      poolHealthPct: 0.824,
      modelHealthPct: 0.95,
      priorityScore: '650',
      matchedRuleIds: [],
      etag: `etag-${index}`,
    });
  }

  withTaskId(taskId: string) {
    this.item.taskId = taskId;
    return this;
  }

  withDutId(dutId: string) {
    this.item.dutId = dutId;
    this.item.deviceId = dutId;
    return this;
  }

  withDeviceId(deviceId: string) {
    this.item.deviceId = deviceId;
    return this;
  }

  withPool(pool: string) {
    this.item.pools = [pool];
    return this;
  }

  withModel(model: string) {
    this.item.model = model;
    return this;
  }

  withState(state: string) {
    this.item.state = state;
    return this;
  }

  withHealth(poolHealthPct: number, modelHealthPct: number) {
    this.item.poolHealthPct = poolHealthPct;
    this.item.modelHealthPct = modelHealthPct;
    return this;
  }

  withInRepairHealth(poolInRepairPct: number, modelInRepairPct: number) {
    this.item.poolInRepairPct = poolInRepairPct;
    this.item.modelInRepairPct = modelInRepairPct;
    return this;
  }

  withActiveIrmIds(irmIds: string[]) {
    this.item.activeIrmIds = [...irmIds];
    return this;
  }

  withScore(score: number | string) {
    this.item.priorityScore = String(score);
    return this;
  }

  withMatchedRuleIds(ruleIds: string[]) {
    this.item.matchedRuleIds = [...ruleIds];
    return this;
  }

  claimedBy(email: string, claimedAt = '2026-08-19T10:00:00Z') {
    this.item.claimedBy = email;
    this.item.claimedAt = claimedAt;
    return this;
  }

  unclaimed() {
    this.item.claimedBy = '';
    this.item.claimedAt = undefined;
    return this;
  }

  build(): RepairQueueItem {
    return { ...this.item };
  }
}

export const aRepairQueueItem = (index = 1) =>
  new RepairQueueItemBuilder(index);

export const aPriorityRule = (
  id: string,
  expressionAip160: string,
  weight: string,
): PriorityRule =>
  PriorityRule.fromPartial({
    id,
    expressionAip160,
    weight,
  });
