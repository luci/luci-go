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

export type HealthStatusBand = 'Healthy' | 'Warning' | 'Critical';
export type HealthStatusColor = 'success' | 'warning' | 'error';

/**
 * Single source of truth (SSOT) percentage cutoffs for Fleet Health SLA tiers.
 */
export const HEALTHY_THRESHOLD_PCT = 95;
export const WARNING_THRESHOLD_PCT = 80;

export interface HealthThresholdDefinition {
  readonly status: HealthStatusBand;
  readonly label: string;
  /** Minimum percentage (inclusive) required to meet this health tier. */
  readonly minPct: number;
  /** Maximum percentage (inclusive) for this tier, or undefined if no upper bound. */
  readonly maxPct?: number;
  readonly color: HealthStatusColor;
  readonly colorSx: string;
}

/**
 * Canonical SLA threshold definitions ordered from highest to lowest standard.
 */
export const HEALTH_THRESHOLD_DEFINITIONS: readonly HealthThresholdDefinition[] =
  [
    {
      status: 'Healthy',
      label: 'Healthy',
      minPct: HEALTHY_THRESHOLD_PCT,
      color: 'success',
      colorSx: 'success.main',
    },
    {
      status: 'Warning',
      label: 'Warning',
      minPct: WARNING_THRESHOLD_PCT,
      maxPct: HEALTHY_THRESHOLD_PCT - 1,
      color: 'warning',
      colorSx: 'warning.main',
    },
    {
      status: 'Critical',
      label: 'Critical',
      minPct: 0,
      maxPct: WARNING_THRESHOLD_PCT - 1,
      color: 'error',
      colorSx: 'error.main',
    },
  ];

export interface HealthThreshold {
  readonly status: HealthStatusBand;
  readonly label: string;
  readonly range: string;
  readonly color: HealthStatusColor;
  readonly colorSx: string;
}

/** Formats the percentage range string for a threshold definition from the SSOT cutoffs. */
export const formatThresholdRange = (
  def: HealthThresholdDefinition,
): string => {
  if (def.status === 'Healthy') {
    return `≥ ${def.minPct}%`;
  }
  if (def.status === 'Warning') {
    return `${def.minPct}%–${def.maxPct}%`;
  }
  return `< ${WARNING_THRESHOLD_PCT}%`;
};

/** Derived list of threshold items with formatted range strings for display. */
export const HEALTH_THRESHOLDS: readonly HealthThreshold[] =
  HEALTH_THRESHOLD_DEFINITIONS.map((def) => ({
    status: def.status,
    label: def.label,
    range: formatThresholdRange(def),
    color: def.color,
    colorSx: def.colorSx,
  }));

/**
 * Maps availability / health percentage to SLA status bands:
 * - Healthy: >= 95% (success)
 * - Warning: 80% - 94% (warning)
 * - Critical: < 80% (error)
 */
export const getHealthStatus = (
  pct: number,
): { status: HealthStatusBand; color: HealthStatusColor } => {
  for (const def of HEALTH_THRESHOLD_DEFINITIONS) {
    if (pct >= def.minPct) {
      return { status: def.status, color: def.color };
    }
  }
  return { status: 'Critical', color: 'error' };
};
