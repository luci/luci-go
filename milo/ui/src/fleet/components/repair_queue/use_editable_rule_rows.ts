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

import { useEffect, useState } from 'react';

import {
  EditableRuleRow,
  isRowDirty,
  RepairPriorityRule,
  ruleToEditableRow,
} from './editable_rule_row';

/**
 * Keeps a list of locally editable rule rows in sync with the remote rules,
 * preserving local edits on dirty rows and any unsaved draft rows.
 */
export const useEditableRuleRows = (rules: readonly RepairPriorityRule[]) => {
  const [rows, setRows] = useState<readonly EditableRuleRow[]>([]);

  // Sync rows with remote rules
  useEffect(() => {
    setRows((prevRows) => {
      if (prevRows.length === 0) {
        return rules.map(ruleToEditableRow);
      }

      const updatedRemoteRows: EditableRuleRow[] = rules.map((rule) => {
        const existing = prevRows.find((r) => r.id === rule.id);
        if (!existing) {
          return ruleToEditableRow(rule);
        }
        if (isRowDirty(existing)) {
          return {
            ...existing,
            backendExpressionAip160: rule.expressionAip160,
            backendWeight: rule.weight,
          };
        }
        return ruleToEditableRow(rule);
      });

      const activeDraftRows = prevRows.filter((r) => r.isDraft);
      return [...updatedRemoteRows, ...activeDraftRows];
    });
  }, [rules]);

  return [rows, setRows] as const;
};
