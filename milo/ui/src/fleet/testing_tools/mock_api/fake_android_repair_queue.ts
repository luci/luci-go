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
  AndroidRepairQueueItem,
  AndroidWorkspace,
  ClaimAndroidRepairTaskRequest,
  ClaimAndroidRepairTaskResponse,
  FleetConsole,
  ListAndroidRepairQueueRequest,
  ListAndroidRepairQueueResponse,
  UnclaimAndroidRepairTaskRequest,
  UnclaimAndroidRepairTaskResponse,
} from '../../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { createAndroidPriorityRulesFake } from './fake_android_priority_rules';
import { Aip160Matcher, rpc, RpcJsonHandler } from './fake_priority_rules';

export function createAndroidRepairQueueFake(deps: {
  androidPriorityRules: ReturnType<typeof createAndroidPriorityRulesFake>;
  matchesFilter: Aip160Matcher;
  getCurrentUserEmail: () => string;
}) {
  let items: AndroidRepairQueueItem[] = [];
  let explicitTotalSize: number | undefined;
  let explicitNextPageToken = '';

  const toFilterableItem = (
    item: AndroidRepairQueueItem,
  ): Record<string, unknown> => {
    const effectiveHostGroupHealthPct =
      item.hostGroupHealthPct !== undefined
        ? Math.min(
            1,
            item.hostGroupHealthPct + (item.hostGroupInRepairPct ?? 0),
          )
        : undefined;
    const effectiveRunTargetHealthPct =
      item.runTargetHealthPct !== undefined
        ? Math.min(
            1,
            item.runTargetHealthPct + (item.runTargetInRepairPct ?? 0),
          )
        : undefined;
    return {
      ...(AndroidRepairQueueItem.toJSON(item) as Record<string, unknown>),
      device_id: item.deviceId,
      host_group: item.hostGroups,
      host_groups: item.hostGroups,
      run_target: item.runTarget,
      lab_name: item.labName,
      state: item.state,
      claimed_by: item.claimedBy ?? '',
      fc_machine_type:
        item.fcMachineType === AndroidMachineType.ANDROID_MACHINE_TYPE_HOST
          ? 'host'
          : 'device',
      host_group_health_pct: item.hostGroupHealthPct,
      run_target_health_pct: item.runTargetHealthPct,
      effective_host_group_health_pct: effectiveHostGroupHealthPct,
      effective_run_target_health_pct: effectiveRunTargetHealthPct,
    };
  };

  const scoreItem = (
    item: AndroidRepairQueueItem,
    idx: number,
    workspace: AndroidWorkspace,
    siteId: string,
  ): AndroidRepairQueueItem => {
    const rank = item.rank || idx + 1;
    const { hasRules, totalScore, matchedRuleIds } =
      deps.androidPriorityRules.evaluateItemScore(
        toFilterableItem(item),
        workspace,
        siteId,
      );
    if (!hasRules) {
      return AndroidRepairQueueItem.create({ ...item, rank });
    }
    return AndroidRepairQueueItem.create({
      ...item,
      rank,
      priorityScore: String(totalScore),
      matchedRuleIds,
    });
  };

  const pinCallerClaimedFirst = (
    rows: readonly AndroidRepairQueueItem[],
  ): AndroidRepairQueueItem[] => {
    const callerEmail = deps.getCurrentUserEmail();
    if (!callerEmail) {
      return [...rows];
    }
    const pinned: AndroidRepairQueueItem[] = [];
    const rest: AndroidRepairQueueItem[] = [];
    for (const row of rows) {
      (row.claimedBy === callerEmail ? pinned : rest).push(row);
    }
    return [...pinned, ...rest];
  };

  const service: Pick<
    FleetConsole,
    | 'ListAndroidRepairQueue'
    | 'ClaimAndroidRepairTask'
    | 'UnclaimAndroidRepairTask'
  > = {
    async ListAndroidRepairQueue(req: ListAndroidRepairQueueRequest) {
      let scored = items.map((item, idx) =>
        scoreItem(item, idx, req.workspace, req.siteId),
      );
      const inProgressCount = scored.filter((i) => Boolean(i.claimedBy)).length;
      if (req.filter) {
        scored = scored.filter((item) =>
          deps.matchesFilter(toFilterableItem(item), req.filter),
        );
      }
      const ordered = pinCallerClaimedFirst(scored);
      const totalSize =
        !req.filter && explicitTotalSize !== undefined
          ? explicitTotalSize
          : ordered.length;
      const startIndex = req.pageToken?.startsWith('token-')
        ? parseInt(req.pageToken.replace('token-', ''), 10) || 0
        : 0;
      const pageSize = req.pageSize > 0 ? req.pageSize : ordered.length;
      const paged = ordered.slice(startIndex, startIndex + pageSize);
      const computedNext =
        startIndex + pageSize < ordered.length
          ? `token-${startIndex + pageSize}`
          : '';
      const nextPageToken =
        !req.filter && explicitNextPageToken
          ? explicitNextPageToken
          : computedNext;

      return ListAndroidRepairQueueResponse.create({
        repairQueueItems: paged,
        totalSize,
        nextPageToken,
        inProgressCount,
      });
    },

    async ClaimAndroidRepairTask(req: ClaimAndroidRepairTaskRequest) {
      const email = deps.getCurrentUserEmail() || 'user@example.com';
      const nowIso = new Date().toISOString();
      let updated: AndroidRepairQueueItem | undefined;
      items = items.map((item) => {
        if (item.taskId === req.taskId) {
          updated = AndroidRepairQueueItem.create({
            ...item,
            claimedBy: email,
            claimedAt: nowIso,
          });
          return updated;
        }
        return item;
      });
      return ClaimAndroidRepairTaskResponse.create({
        repairQueueItem:
          updated ??
          AndroidRepairQueueItem.create({
            taskId: req.taskId,
            claimedBy: email,
            claimedAt: nowIso,
          }),
      });
    },

    async UnclaimAndroidRepairTask(req: UnclaimAndroidRepairTaskRequest) {
      let updated: AndroidRepairQueueItem | undefined;
      items = items.map((item) => {
        if (item.taskId === req.taskId) {
          updated = AndroidRepairQueueItem.create({
            ...item,
            claimedBy: '',
            claimedAt: undefined,
          });
          return updated;
        }
        return item;
      });
      return UnclaimAndroidRepairTaskResponse.create({
        repairQueueItem:
          updated ??
          AndroidRepairQueueItem.create({
            taskId: req.taskId,
            claimedBy: '',
          }),
      });
    },
  };

  return {
    ...service,
    handlers: {
      ListAndroidRepairQueue: rpc(
        ListAndroidRepairQueueRequest,
        ListAndroidRepairQueueResponse,
        service.ListAndroidRepairQueue,
      ),
      ClaimAndroidRepairTask: rpc(
        ClaimAndroidRepairTaskRequest,
        ClaimAndroidRepairTaskResponse,
        service.ClaimAndroidRepairTask,
      ),
      UnclaimAndroidRepairTask: rpc(
        UnclaimAndroidRepairTaskRequest,
        UnclaimAndroidRepairTaskResponse,
        service.UnclaimAndroidRepairTask,
      ),
    } satisfies Record<string, RpcJsonHandler>,
    seed(res: Partial<ListAndroidRepairQueueResponse>) {
      items = (res.repairQueueItems ?? []).map((i) =>
        AndroidRepairQueueItem.create(i),
      );
      explicitTotalSize = res.totalSize;
      explicitNextPageToken = res.nextPageToken ?? '';
    },
  };
}
