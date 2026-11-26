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
  AndroidMachineType,
  AndroidPriorityRule,
  AndroidRepairQueueItem,
  AndroidWorkspace,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const DEFAULT_ANDROID_REPAIR_QUEUE_DIMENSIONS = {
  baseDimensions: {
    id: { values: ['android-device-101', 'android-device-102'] },
    fc_machine_type: { values: ['device', 'host'] },
    lab_name: { values: ['MTV-1', 'MTV-2'] },
    host_group: { values: ['presubmit_critical', 'crystalball_perf'] },
    run_target: { values: ['seahawk', 'mustang', 'komodo'] },
    state: { values: ['MISSING', 'DYING', 'FAILED'] },
  },
  labels: {},
};

type Mutable<T> = {
  -readonly [P in keyof T]: T[P];
};

/**
 * Fluent test data builder for `AndroidRepairQueueItem` so each integration
 * test can specify only the minimal fields relevant to its scenario.
 */
export class AndroidRepairQueueItemBuilder {
  private item: Mutable<AndroidRepairQueueItem>;

  constructor(index = 1) {
    this.item = AndroidRepairQueueItem.fromPartial({
      taskId: `${100 + index}`,
      deviceId: `android-device-${100 + index}`,
      fcMachineType: AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE,
      hostGroups: ['presubmit_critical'],
      runTarget: 'seahawk',
      labName: 'MTV-1',
      state: 'MISSING',
      claimedBy: '',
      claimedAt: undefined,
      hostGroupHealthPct: 0.824,
      runTargetHealthPct: 0.95,
      priorityScore: '250',
      matchedRuleIds: [],
      etag: `etag-${100 + index}`,
      rank: index,
    });
  }

  withTaskId(taskId: string) {
    this.item.taskId = taskId;
    return this;
  }

  withDeviceId(deviceId: string) {
    this.item.deviceId = deviceId;
    return this;
  }

  asHost() {
    this.item.fcMachineType = AndroidMachineType.ANDROID_MACHINE_TYPE_HOST;
    this.item.runTarget = '';
    this.item.runTargetHealthPct = undefined;
    this.item.runTargetInRepairPct = undefined;
    return this;
  }

  withHostGroups(hostGroups: string[]) {
    this.item.hostGroups = [...hostGroups];
    return this;
  }

  withRunTarget(runTarget: string) {
    this.item.runTarget = runTarget;
    return this;
  }

  withLab(labName: string) {
    this.item.labName = labName;
    return this;
  }

  withState(state: string) {
    this.item.state = state;
    return this;
  }

  withHealth(
    hostGroupHealthPct: number | undefined,
    runTargetHealthPct: number | undefined,
  ) {
    this.item.hostGroupHealthPct = hostGroupHealthPct;
    this.item.runTargetHealthPct = runTargetHealthPct;
    return this;
  }

  withInRepairHealth(
    hostGroupInRepairPct: number | undefined,
    runTargetInRepairPct: number | undefined,
  ) {
    this.item.hostGroupInRepairPct = hostGroupInRepairPct;
    this.item.runTargetInRepairPct = runTargetInRepairPct;
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

  withRank(rank: number) {
    this.item.rank = rank;
    return this;
  }

  claimedBy(email: string, claimedAt = '2026-09-16T10:00:00Z') {
    this.item.claimedBy = email;
    this.item.claimedAt = claimedAt;
    return this;
  }

  unclaimed() {
    this.item.claimedBy = '';
    this.item.claimedAt = undefined;
    return this;
  }

  build(): AndroidRepairQueueItem {
    return AndroidRepairQueueItem.create(this.item);
  }
}

export const anAndroidRepairQueueItem = (index = 1) =>
  new AndroidRepairQueueItemBuilder(index);

export const anAndroidPriorityRule = (
  id: string,
  expressionAip160: string,
  weight: string | number,
  workspace: AndroidWorkspace = AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
  siteId = '',
): AndroidPriorityRule =>
  AndroidPriorityRule.create({
    id,
    expressionAip160,
    weight: String(weight),
    workspace,
    siteId,
  });
