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
  ClaimRepairTaskRequest,
  ClaimRepairTaskResponse,
  FleetConsole,
  ListRepairQueueRequest,
  ListRepairQueueResponse,
  RepairQueueItem,
  UnclaimRepairTaskRequest,
  UnclaimRepairTaskResponse,
} from '../../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  Aip160Matcher,
  createPriorityRulesFake,
  rpc,
  RpcJsonHandler,
} from './fake_priority_rules';

export function createRepairQueueFake(deps: {
  priorityRules: ReturnType<typeof createPriorityRulesFake>;
  matchesFilter: Aip160Matcher;
  getCurrentUserEmail: () => string;
}) {
  let items: RepairQueueItem[] = [];
  let explicitTotalSize: number | undefined;
  let explicitNextPageToken = '';

  const toFilterableItem = (
    item: RepairQueueItem,
  ): Record<string, unknown> => ({
    ...(RepairQueueItem.toJSON(item) as Record<string, unknown>),
    deviceSpec: {
      labels: {
        'label-pool': { values: item.pools },
      },
    },
  });

  const scoreItem = (item: RepairQueueItem): RepairQueueItem => {
    if (deps.priorityRules.rules.size === 0) {
      return RepairQueueItem.create(item);
    }
    const { totalScore, matchedRuleIds } = deps.priorityRules.evaluateItemScore(
      toFilterableItem(item),
    );
    return RepairQueueItem.create({
      ...item,
      priorityScore: String(totalScore),
      matchedRuleIds,
    });
  };

  const service: Pick<
    FleetConsole,
    'ListRepairQueue' | 'ClaimRepairTask' | 'UnclaimRepairTask'
  > = {
    async ListRepairQueue(req: ListRepairQueueRequest) {
      let scored = items.map(scoreItem);
      if (req.filter) {
        scored = scored.filter((item) =>
          deps.matchesFilter(toFilterableItem(item), req.filter),
        );
      }
      const inProgressCount = scored.filter((i) => Boolean(i.claimedBy)).length;
      const totalSize =
        !req.filter && explicitTotalSize !== undefined
          ? explicitTotalSize
          : scored.length;
      const startIndex = req.pageToken?.startsWith('token-')
        ? parseInt(req.pageToken.replace('token-', ''), 10) || 0
        : 0;
      const pageSize = req.pageSize > 0 ? req.pageSize : scored.length;
      const paged = scored.slice(startIndex, startIndex + pageSize);
      const computedNext =
        startIndex + pageSize < scored.length
          ? `token-${startIndex + pageSize}`
          : '';
      const nextPageToken =
        !req.filter && explicitNextPageToken
          ? explicitNextPageToken
          : computedNext;

      return ListRepairQueueResponse.create({
        repairQueueItems: paged,
        totalSize,
        nextPageToken,
        inProgressCount,
      });
    },

    async ClaimRepairTask(req: ClaimRepairTaskRequest) {
      const email = deps.getCurrentUserEmail() || 'user@example.com';
      const nowIso = new Date().toISOString();
      let updated: RepairQueueItem | undefined;
      items = items.map((item) => {
        if (item.taskId === req.taskId) {
          updated = RepairQueueItem.create({
            ...item,
            claimedBy: email,
            claimedAt: nowIso,
          });
          return updated;
        }
        return item;
      });
      return ClaimRepairTaskResponse.create({
        repairQueueItem:
          updated ??
          RepairQueueItem.create({
            taskId: req.taskId,
            claimedBy: email,
            claimedAt: nowIso,
          }),
      });
    },

    async UnclaimRepairTask(req: UnclaimRepairTaskRequest) {
      let updated: RepairQueueItem | undefined;
      items = items.map((item) => {
        if (item.taskId === req.taskId) {
          updated = RepairQueueItem.create({
            ...item,
            claimedBy: '',
            claimedAt: undefined,
          });
          return updated;
        }
        return item;
      });
      return UnclaimRepairTaskResponse.create({
        repairQueueItem:
          updated ??
          RepairQueueItem.create({ taskId: req.taskId, claimedBy: '' }),
      });
    },
  };

  return {
    ...service,
    handlers: {
      ListRepairQueue: rpc(
        ListRepairQueueRequest,
        ListRepairQueueResponse,
        service.ListRepairQueue,
      ),
      ClaimRepairTask: rpc(
        ClaimRepairTaskRequest,
        ClaimRepairTaskResponse,
        service.ClaimRepairTask,
      ),
      UnclaimRepairTask: rpc(
        UnclaimRepairTaskRequest,
        UnclaimRepairTaskResponse,
        service.UnclaimRepairTask,
      ),
    } satisfies Record<string, RpcJsonHandler>,
    getSnapshot() {
      const scored = items.map(scoreItem);
      const inProgressCount = scored.filter((i) => Boolean(i.claimedBy)).length;
      return ListRepairQueueResponse.create({
        repairQueueItems: scored,
        totalSize:
          explicitTotalSize !== undefined ? explicitTotalSize : scored.length,
        nextPageToken: explicitNextPageToken,
        inProgressCount,
      });
    },
    seed(res: Partial<ListRepairQueueResponse>) {
      items = (res.repairQueueItems ?? []).map((i) =>
        RepairQueueItem.create(i),
      );
      explicitTotalSize = res.totalSize;
      explicitNextPageToken = res.nextPageToken ?? '';
    },
  };
}
