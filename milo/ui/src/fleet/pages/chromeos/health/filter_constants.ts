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

export interface HealthFilterConfig {
  readonly key: string;
  readonly label: string;
  readonly dimensionSource: string;
  readonly supersededLabelKeys: readonly string[];
}

/**
 * Single source of truth (SSOT) for core filters recognized by the
 * ChromeOS Health Metrics dashboard.
 */
export const HEALTH_FILTER_CONFIGS = {
  MODEL: {
    key: 'model',
    label: 'Model',
    dimensionSource: 'label-model',
    supersededLabelKeys: ['label-model', 'model'],
  },
  POOL: {
    key: 'pool',
    label: 'Pool',
    dimensionSource: 'label-pool',
    supersededLabelKeys: ['label-pool', 'pool', 'pools'],
  },
} as const;

export type HealthFilterConfigKey = keyof typeof HEALTH_FILTER_CONFIGS;
export type HealthFilterKey =
  | (typeof HEALTH_FILTER_CONFIGS)[HealthFilterConfigKey]['key']
  | string;
