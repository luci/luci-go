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
  dutState,
  formatDutStateLabel,
} from '@/fleet/pages/device_list_page/chromeos/dut_state';
import { healthStateColors } from '@/fleet/theme/tokens';

/**
 * Exact ChromeOS DUT state keys (lowercase, matching AIP-160 labels."dut_state" values).
 * Derived directly from canonical dutState enum to avoid string duplication.
 */
export type ExactDutStateKey = Lowercase<dutState>;

/**
 * Health Dashboard Macro-Categories.
 *
 * NOTE - Architectural contrast between Devices List and Health Dashboard:
 * In Devices List (dut_state.ts), state badges use discrete semantic colors
 * (e.g. NEEDS_REPAIR is orange, RESERVED is purple) to identify individual DUT rows.
 * In Health Dashboard, however, the primary objective is rapid, pre-attentive
 * evaluation of Fleet Capacity and Actionability on a continuous progress bar:
 * - Healthy (Green spectrum): Devices that represent usable or self-healing capacity
 *   (ready, needs_repair, repair_failed, reserved). Operators immediately see what
 *   fraction of the fleet is operable without human lab intervention.
 * - Unhealthy (Red/Amber spectrum): Devices requiring physical lab technician
 *   intervention or deployment qualification (needs_deploy, needs_manual_repair,
 *   needs_replacement).
 * - Other states (Grey spectrum): Inventory or unprovisioned staging devices
 *   (registered, unknown).
 */
export type HealthMacroCategory =
  | 'healthy'
  | 'action_required'
  | 'administrative';

export interface HealthCategoryConfig {
  readonly label: string;
  readonly color: string;
  readonly states: readonly ExactDutStateKey[];
}

export const HEALTH_CATEGORIES: Record<
  HealthMacroCategory,
  HealthCategoryConfig
> = {
  healthy: {
    label: 'Healthy (Operable Capacity)',
    color: healthStateColors.macroHealthy,
    states: ['reserved', 'ready', 'needs_repair', 'repair_failed'],
  },
  action_required: {
    label: 'Unhealthy (Action Required)',
    color: healthStateColors.macroActionRequired,
    states: ['needs_deploy', 'needs_manual_repair', 'needs_replacement'],
  },
  administrative: {
    label: 'Other states',
    color: healthStateColors.macroAdministrative,
    states: ['registered', 'unknown'],
  },
};

export interface StateVisualConfig {
  readonly key: ExactDutStateKey;
  readonly label: string;
  readonly color: string;
  readonly description: string;
}

export const STATE_VISUAL_CONFIGS: Record<ExactDutStateKey, StateVisualConfig> =
  {
    reserved: {
      key: 'reserved',
      label: formatDutStateLabel(dutState.RESERVED),
      color: healthStateColors.reserved, // Dark Green (#1b5e20)
      description: 'Reserved or leased by user; hardware healthy and active',
    },
    ready: {
      key: 'ready',
      label: formatDutStateLabel(dutState.READY),
      color: healthStateColors.ready, // Medium Green (#43a047)
      description: 'Serving tests or idle and immediately available',
    },
    needs_repair: {
      key: 'needs_repair',
      label: formatDutStateLabel(dutState.NEEDS_REPAIR),
      color: healthStateColors.needsRepair, // Light Green (#81c784)
      description: 'Active automated repair task running in lab automation',
    },
    repair_failed: {
      key: 'repair_failed',
      label: formatDutStateLabel(dutState.REPAIR_FAILED),
      color: healthStateColors.repairFailed, // Light Green (#81c784)
      description:
        'Initial automated repair failed; queued for automated retry',
    },
    needs_deploy: {
      key: 'needs_deploy',
      label: formatDutStateLabel(dutState.NEEDS_DEPLOY),
      color: healthStateColors.needsDeploy, // Red (#d32f2f)
      description:
        'New hardware physically racked, awaiting qualification deployment',
    },
    needs_manual_repair: {
      key: 'needs_manual_repair',
      label: formatDutStateLabel(dutState.NEEDS_MANUAL_REPAIR),
      color: healthStateColors.needsManualRepair, // Red (#d32f2f)
      description:
        'Automation exhausted; physical hardware technician intervention needed',
    },
    needs_replacement: {
      key: 'needs_replacement',
      label: formatDutStateLabel(dutState.NEEDS_REPLACEMENT),
      color: healthStateColors.needsReplacement, // Red (#d32f2f)
      description:
        'Permanent hardware failure; unit must be replaced with spare',
    },
    registered: {
      key: 'registered',
      label: formatDutStateLabel(dutState.REGISTERED),
      color: healthStateColors.registered,
      description: 'Registered in inventory database but not yet provisioned',
    },
    unknown: {
      key: 'unknown',
      label: formatDutStateLabel(dutState.UNKNOWN),
      color: healthStateColors.unknown,
      description: 'State could not be determined or blank',
    },
  };
