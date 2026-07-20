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

import { fromString } from './from_string';
import { toString as idToString } from './to_string';
import { root } from './wrap';

/**
 * Resolves the base Check or Stage node ID from a potentially detailed identifier string
 * (e.g. one containing type prefixes and suffixes like attempts or results).
 *
 * This is used to map URL-level selection parameters back to the corresponding Check/Stage
 * node represented on the graph canvas.
 *
 * Examples:
 * - 'S$init:A1' (StageAttempt) -> '$init'
 * - 'Ccheck_id:R1' (CheckResult) -> 'check_id'
 * - 'Lworkplan_id:Nstage_id' (Full Stage) -> 'stage_id'
 * - 'stage_without_prefix' (Raw ID) -> 'stage_without_prefix'
 */
export function getBaseNodeId(
  nodeId: string | undefined,
  options?: { readonly includePrefix?: boolean },
): string | undefined {
  if (!nodeId) return undefined;
  try {
    const parsed = fromString(nodeId);
    const resolved = root(parsed);
    const includePrefix = options?.includePrefix !== false;
    if (includePrefix) {
      if (resolved.check) {
        return idToString({ check: resolved.check }, { omitWorkPlan: true });
      }
      if (resolved.stage) {
        return idToString({ stage: resolved.stage }, { omitWorkPlan: true });
      }
    }
    return resolved.check?.id || resolved.stage?.id || nodeId;
  } catch {
    return nodeId;
  }
}
