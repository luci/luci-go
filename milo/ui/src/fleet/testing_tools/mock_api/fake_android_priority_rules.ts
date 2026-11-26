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
  AndroidPriorityRule,
  AndroidWorkspace,
  CreateAndroidPriorityRuleRequest,
  CreateAndroidPriorityRuleResponse,
  DeleteAndroidPriorityRuleRequest,
  DeleteAndroidPriorityRuleResponse,
  FleetConsole,
  ListAndroidPriorityRulesRequest,
  ListAndroidPriorityRulesResponse,
  UpdateAndroidPriorityRuleRequest,
  UpdateAndroidPriorityRuleResponse,
} from '../../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { Aip160Matcher, rpc, RpcJsonHandler } from './fake_priority_rules';

export function createAndroidPriorityRulesFake(matchesFilter: Aip160Matcher) {
  const rules = new Map<string, AndroidPriorityRule>();
  let nextRuleId = 1;

  const matchingRules = (
    workspace: AndroidWorkspace,
    siteId = '',
  ): AndroidPriorityRule[] => {
    const normalizedSite = siteId.trim().toLowerCase();
    return Array.from(rules.values()).filter(
      (r) =>
        (workspace === AndroidWorkspace.ANDROID_WORKSPACE_UNSPECIFIED ||
          r.workspace === workspace) &&
        r.siteId.trim().toLowerCase() === normalizedSite,
    );
  };

  const service: Pick<
    FleetConsole,
    | 'ListAndroidPriorityRules'
    | 'CreateAndroidPriorityRule'
    | 'UpdateAndroidPriorityRule'
    | 'DeleteAndroidPriorityRule'
  > = {
    async ListAndroidPriorityRules(req: ListAndroidPriorityRulesRequest) {
      return ListAndroidPriorityRulesResponse.create({
        priorityRules: matchingRules(req.workspace, req.siteId),
      });
    },

    async CreateAndroidPriorityRule(req: CreateAndroidPriorityRuleRequest) {
      const incoming = req.priorityRule;
      const id =
        incoming?.id && incoming.id !== '0'
          ? incoming.id
          : `rule-${nextRuleId++}`;
      const created = AndroidPriorityRule.create({
        id,
        expressionAip160: incoming?.expressionAip160 ?? '',
        weight: incoming?.weight ?? '0',
        workspace:
          incoming?.workspace ?? AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
        siteId: incoming?.siteId ?? '',
      });
      rules.set(created.id, created);
      return CreateAndroidPriorityRuleResponse.create({
        priorityRule: created,
      });
    },

    async UpdateAndroidPriorityRule(req: UpdateAndroidPriorityRuleRequest) {
      const existing = rules.get(req.id);
      const updated = AndroidPriorityRule.create({
        id: req.id,
        expressionAip160:
          req.expressionAip160 !== undefined
            ? req.expressionAip160
            : (existing?.expressionAip160 ?? ''),
        weight:
          req.weight !== undefined ? req.weight : (existing?.weight ?? '0'),
        workspace:
          existing?.workspace ?? AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
        siteId: existing?.siteId ?? '',
      });
      rules.set(updated.id, updated);
      return UpdateAndroidPriorityRuleResponse.create({
        priorityRule: updated,
      });
    },

    async DeleteAndroidPriorityRule(req: DeleteAndroidPriorityRuleRequest) {
      rules.delete(req.id);
      return DeleteAndroidPriorityRuleResponse.create({});
    },
  };

  return {
    ...service,
    handlers: {
      ListAndroidPriorityRules: rpc(
        ListAndroidPriorityRulesRequest,
        ListAndroidPriorityRulesResponse,
        service.ListAndroidPriorityRules,
      ),
      CreateAndroidPriorityRule: rpc(
        CreateAndroidPriorityRuleRequest,
        CreateAndroidPriorityRuleResponse,
        service.CreateAndroidPriorityRule,
      ),
      UpdateAndroidPriorityRule: rpc(
        UpdateAndroidPriorityRuleRequest,
        UpdateAndroidPriorityRuleResponse,
        service.UpdateAndroidPriorityRule,
      ),
      DeleteAndroidPriorityRule: rpc(
        DeleteAndroidPriorityRuleRequest,
        DeleteAndroidPriorityRuleResponse,
        service.DeleteAndroidPriorityRule,
      ),
    } satisfies Record<string, RpcJsonHandler>,
    rules,
    seed(nextRules: readonly AndroidPriorityRule[]) {
      rules.clear();
      nextRuleId = 1;
      for (const r of nextRules) {
        const normalized = AndroidPriorityRule.create(r);
        rules.set(normalized.id, normalized);
        const match = normalized.id.match(/^rule-(\d+)$/);
        if (match) {
          nextRuleId = Math.max(nextRuleId, Number(match[1]) + 1);
        }
      }
    },
    evaluateItemScore(
      item: Record<string, unknown>,
      workspace: AndroidWorkspace,
      siteId = '',
    ) {
      const scoped = matchingRules(workspace, siteId);
      const matched = scoped.filter(
        (r) => r.expressionAip160 && matchesFilter(item, r.expressionAip160),
      );
      const totalScore = matched.reduce(
        (sum, r) => sum + (parseInt(String(r.weight || '0'), 10) || 0),
        0,
      );
      return {
        hasRules: scoped.length > 0,
        totalScore,
        matchedRuleIds: matched.map((r) => r.id),
      };
    },
  };
}
