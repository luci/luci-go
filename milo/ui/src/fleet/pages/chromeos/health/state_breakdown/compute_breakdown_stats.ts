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
  AverageDeviceStateCounts,
  DeviceStateCounts,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  ExactDutStateKey,
  HEALTH_CATEGORIES,
  HealthMacroCategory,
  STATE_VISUAL_CONFIGS,
  StateVisualConfig,
} from './health_state_definitions';

export interface DeviceStateBreakdown {
  readonly total: number;
  readonly ready: number;
  readonly needsRepair: number;
  readonly repairFailed: number;
  readonly needsManualRepair: number;
  readonly needsReplacement: number;
  readonly reserved: number;
  readonly needsDeploy: number;
  readonly registered?: number;
  readonly unknown?: number;
}

export interface StatePercentageItem {
  readonly config: StateVisualConfig;
  readonly count: number;
  readonly pct: number; // 0 to 100
}

export interface CategorySummaryItem {
  readonly category: HealthMacroCategory;
  readonly label: string;
  readonly color: string;
  readonly count: number;
  readonly pct: number;
  readonly states: StatePercentageItem[];
}

export interface BreakdownStats {
  readonly total: number;
  readonly states: StatePercentageItem[];
  readonly categories: CategorySummaryItem[];
  readonly healthyPct: number;
  readonly recoveringPct: number;
  readonly brokenPct: number;
  readonly adminPct: number;
  readonly healthyTotalCount: number;
  readonly healthyTotalPct: number;
  readonly unhealthyTotalCount: number;
  readonly unhealthyTotalPct: number;
}

/**
 * Converts protobuf AverageDeviceStateCounts or DeviceStateCounts into DeviceStateBreakdown.
 */
export const convertProtoCountsToBreakdown = (
  stateCounts?: AverageDeviceStateCounts | DeviceStateCounts | null,
): DeviceStateBreakdown | undefined => {
  if (!stateCounts) return undefined;
  const ready = stateCounts.ready || 0;
  const needsRepair = stateCounts.needRepair || 0;
  const repairFailed = stateCounts.repairFailed || 0;
  const needsManualRepair = stateCounts.needManualRepair || 0;
  const needsReplacement = stateCounts.needsReplacement || 0;
  const reserved = stateCounts.reserved || 0;
  const needsDeploy = stateCounts.needsDeploy || 0;
  const registered =
    (stateCounts as unknown as Record<string, number>).registered || 0;
  const unknown =
    (stateCounts as unknown as Record<string, number>).unknown || 0;
  const rawTotal =
    ready +
    needsRepair +
    repairFailed +
    needsManualRepair +
    needsReplacement +
    reserved +
    needsDeploy +
    registered +
    unknown;
  if (rawTotal <= 0) return undefined;
  const total = Math.round(rawTotal * 10) / 10;
  return {
    total,
    ready,
    needsRepair,
    repairFailed,
    needsManualRepair,
    needsReplacement,
    reserved,
    needsDeploy,
    registered,
    unknown,
  };
};

/**
 * Formats a device count nicely:
 * - Integer numbers render as whole numbers: 10 -> "10"
 * - Fractional numbers render rounded to 1 decimal place: 0.333 -> "0.3", 1.5 -> "1.5"
 */
export const formatStateCount = (count: number): string => {
  if (Number.isInteger(count)) {
    return count.toLocaleString();
  }
  return Number(count.toFixed(1)).toLocaleString();
};

/**
 * Computes individual state counts, percentages, and category groupings
 * from a raw DeviceStateBreakdown using category-first configuration.
 */
export const computeBreakdownStats = (
  breakdown: DeviceStateBreakdown,
): BreakdownStats => {
  const total = breakdown.total > 0 ? breakdown.total : 1; // avoid divide by zero

  const rawMap: Record<ExactDutStateKey, number> = {
    ready: breakdown.ready,
    needs_repair: breakdown.needsRepair,
    repair_failed: breakdown.repairFailed,
    needs_manual_repair: breakdown.needsManualRepair,
    needs_replacement: breakdown.needsReplacement,
    reserved: breakdown.reserved,
    needs_deploy: breakdown.needsDeploy,
    registered: breakdown.registered ?? 0,
    unknown: breakdown.unknown ?? 0,
  };

  const statesMap: Record<ExactDutStateKey, StatePercentageItem> = {} as Record<
    ExactDutStateKey,
    StatePercentageItem
  >;

  (Object.keys(STATE_VISUAL_CONFIGS) as ExactDutStateKey[]).forEach((key) => {
    const count = rawMap[key] || 0;
    const pct = Math.round((count / total) * 1000) / 10;
    statesMap[key] = {
      config: STATE_VISUAL_CONFIGS[key],
      count,
      pct,
    };
  });

  const categories: CategorySummaryItem[] = (
    Object.keys(HEALTH_CATEGORIES) as HealthMacroCategory[]
  ).map((catKey) => {
    const catConfig = HEALTH_CATEGORIES[catKey];
    const catStates = catConfig.states.map((sKey) => statesMap[sKey]);
    const catCount = catStates.reduce((acc, s) => acc + s.count, 0);
    const catPct = Math.round((catCount / total) * 1000) / 10;
    return {
      category: catKey,
      label: catConfig.label,
      color: catConfig.color,
      count: catCount,
      pct: catPct,
      states: catStates,
    };
  });

  const states = categories.flatMap((c) => c.states);

  const healthyCat = categories.find((c) => c.category === 'healthy');
  const brokenCat = categories.find((c) => c.category === 'action_required');
  const adminCat = categories.find((c) => c.category === 'administrative');

  const healthyTotalCount = healthyCat?.count ?? 0;
  const healthyTotalPct = healthyCat?.pct ?? 0;
  const unhealthyTotalCount = brokenCat?.count ?? 0;
  const unhealthyTotalPct = brokenCat?.pct ?? 0;

  const recoveringCount =
    (rawMap.needs_repair || 0) + (rawMap.repair_failed || 0);
  const recoveringPct = Math.round((recoveringCount / total) * 1000) / 10;

  const readyPct = Math.round(((rawMap.ready || 0) / total) * 1000) / 10;

  return {
    total: breakdown.total,
    states,
    categories,
    healthyPct: readyPct,
    recoveringPct,
    brokenPct: brokenCat?.pct ?? unhealthyTotalPct,
    adminPct: adminCat?.pct ?? 0,
    healthyTotalCount,
    healthyTotalPct,
    unhealthyTotalCount,
    unhealthyTotalPct,
  };
};
