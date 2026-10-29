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
  CreatePriorityRuleRequest,
  CreatePriorityRuleResponse,
  DeletePriorityRuleRequest,
  DeletePriorityRuleResponse,
  FleetConsole,
  ListPriorityRulesRequest,
  ListPriorityRulesResponse,
  PriorityRule,
  UpdatePriorityRuleRequest,
  UpdatePriorityRuleResponse,
} from '../../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export type Aip160Matcher = (
  item: Record<string, unknown>,
  expression: string,
) => boolean;

export type RpcJsonHandler = (raw: unknown) => Promise<Record<string, unknown>>;

export function rpc<Req, Res>(
  requestType: { fromJSON(input: unknown): Req },
  responseType: { toJSON(message: Res): unknown },
  handler: (req: Req) => Promise<Res>,
): RpcJsonHandler {
  return async (raw: unknown) =>
    responseType.toJSON(
      await handler(requestType.fromJSON(raw ?? {})),
    ) as Record<string, unknown>;
}

export function createPriorityRulesFake(matchesFilter: Aip160Matcher) {
  const rules = new Map<string, PriorityRule>();
  let nextRuleId = 1;

  const service: Pick<
    FleetConsole,
    | 'ListPriorityRules'
    | 'CreatePriorityRule'
    | 'UpdatePriorityRule'
    | 'DeletePriorityRule'
  > = {
    async ListPriorityRules(_req: ListPriorityRulesRequest) {
      return ListPriorityRulesResponse.create({
        priorityRules: Array.from(rules.values()),
      });
    },

    async CreatePriorityRule(req: CreatePriorityRuleRequest) {
      const incoming = req.priorityRule;
      const id =
        incoming?.id && incoming.id !== '0'
          ? incoming.id
          : `rule-${nextRuleId++}`;
      const created = PriorityRule.create({
        id,
        expressionAip160: incoming?.expressionAip160 ?? '',
        weight: incoming?.weight ?? '0',
        lab: incoming?.lab ?? '',
      });
      rules.set(created.id, created);
      return CreatePriorityRuleResponse.create({ priorityRule: created });
    },

    async UpdatePriorityRule(req: UpdatePriorityRuleRequest) {
      const existing = rules.get(req.id);
      const updated = PriorityRule.create({
        id: req.id,
        expressionAip160:
          req.expressionAip160 !== undefined
            ? req.expressionAip160
            : (existing?.expressionAip160 ?? ''),
        weight:
          req.weight !== undefined ? req.weight : (existing?.weight ?? '0'),
        lab: existing?.lab ?? '',
      });
      rules.set(updated.id, updated);
      return UpdatePriorityRuleResponse.create({ priorityRule: updated });
    },

    async DeletePriorityRule(req: DeletePriorityRuleRequest) {
      rules.delete(req.id);
      return DeletePriorityRuleResponse.create({});
    },
  };

  return {
    ...service,
    handlers: {
      ListPriorityRules: rpc(
        ListPriorityRulesRequest,
        ListPriorityRulesResponse,
        service.ListPriorityRules,
      ),
      CreatePriorityRule: rpc(
        CreatePriorityRuleRequest,
        CreatePriorityRuleResponse,
        service.CreatePriorityRule,
      ),
      UpdatePriorityRule: rpc(
        UpdatePriorityRuleRequest,
        UpdatePriorityRuleResponse,
        service.UpdatePriorityRule,
      ),
      DeletePriorityRule: rpc(
        DeletePriorityRuleRequest,
        DeletePriorityRuleResponse,
        service.DeletePriorityRule,
      ),
    } satisfies Record<string, RpcJsonHandler>,
    rules,
    seed(nextRules: readonly PriorityRule[]) {
      rules.clear();
      nextRuleId = 1;
      for (const r of nextRules) {
        const normalized = PriorityRule.create(r);
        rules.set(normalized.id, normalized);
        const match = normalized.id.match(/^rule-(\d+)$/);
        if (match) {
          nextRuleId = Math.max(nextRuleId, Number(match[1]) + 1);
        }
      }
    },
    evaluateItemScore(item: Record<string, unknown>) {
      const matched = Array.from(rules.values()).filter(
        (r) => r.expressionAip160 && matchesFilter(item, r.expressionAip160),
      );
      const totalScore = matched.reduce(
        (sum, r) => sum + (parseInt(String(r.weight || '0'), 10) || 0),
        0,
      );
      return { totalScore, matchedRuleIds: matched.map((r) => r.id) };
    },
  };
}
