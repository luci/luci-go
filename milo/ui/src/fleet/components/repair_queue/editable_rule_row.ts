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

export const MIN_RULE_WEIGHT = -1000000;
export const MAX_RULE_WEIGHT = 1000000;

/**
 * The fields of a priority rule that the repair queue UI reads and edits.
 * Structurally compatible with the per-platform priority rule protos.
 */
export interface RepairPriorityRule {
  readonly id: string;
  readonly expressionAip160: string;
  readonly weight: string;
}

export interface EditableRuleRow {
  readonly id: string;
  readonly isDraft: boolean;
  readonly expressionAip160: string;
  readonly weight: string;
  readonly backendExpressionAip160: string;
  readonly backendWeight: string;
}

export const ruleToEditableRow = (
  rule: RepairPriorityRule,
): EditableRuleRow => ({
  id: rule.id,
  isDraft: false,
  expressionAip160: rule.expressionAip160,
  weight: rule.weight,
  backendExpressionAip160: rule.expressionAip160,
  backendWeight: rule.weight,
});

export const isRowDirty = (row: EditableRuleRow): boolean => {
  if (row.isDraft) return true;
  return (
    row.expressionAip160 !== row.backendExpressionAip160 ||
    row.weight !== row.backendWeight
  );
};
