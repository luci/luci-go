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

import { createFeatureFlag } from '@/common/feature_flags';

export const enableAndroidUtilizationMetrics = createFeatureFlag({
  description:
    'Displays average Android device utilization metrics, columns, and filters.',
  namespace: 'fleet-console',
  name: 'android-utilization-metrics',
  percentage: {
    dev: 100,
    prod: 0,
  },
  trackingBug: '525089400',
  allowedEnvironments: ['dev', 'prod'],
});

export const enablePTE = createFeatureFlag({
  description: 'Enables PTE support in the fleet console.',
  namespace: 'fleet-console',
  name: 'pte-support',
  percentage: 100,
  trackingBug: '503760268',
  allowedEnvironments: ['dev', 'prod'],
});

export const enableChromeOsRepairsDashboard = createFeatureFlag({
  description: 'Displays the ChromeOS Repair Dashboard shell.',
  namespace: 'fleet-console',
  name: 'chromeos-repairs-dashboard',
  percentage: {
    dev: 100,
    prod: 0,
  },
  trackingBug: '542600108',
  allowedEnvironments: ['dev', 'prod'],
});

export const enableAndroidHealthMetrics = createFeatureFlag({
  description:
    'Displays unified Android device health metrics based on health categories.',
  namespace: 'fleet-console',
  name: 'android-health-metrics',
  percentage: 100,
  trackingBug: '537412303',
  allowedEnvironments: ['dev', 'prod'],
});

export const enableChromeOsWorkforceActivity = createFeatureFlag({
  description:
    'Displays the ChromeOS Workforce Activity monitoring view in repairs.',
  namespace: 'fleet-console',
  name: 'chromeos-workforce-activity',
  percentage: {
    dev: 100,
    prod: 0,
  },
  allowedEnvironments: ['dev'],
});

export const enableWorkforcePickupRank = createFeatureFlag({
  description:
    'Displays the Avg Queue Pickup Rank metric and table column in ChromeOS Workforce Activity.',
  namespace: 'fleet-console',
  name: 'workforce-avg-queue-pickup-rank',
  percentage: {
    dev: 0,
    prod: 0,
  },
  allowedEnvironments: ['dev'],
});

export const enableWorkforceMttr = createFeatureFlag({
  description:
    'Displays the Avg Duration / MTTR metric and table column in ChromeOS Workforce Activity.',
  namespace: 'fleet-console',
  name: 'workforce-avg-duration-mttr',
  percentage: {
    dev: 0,
    prod: 0,
  },
  allowedEnvironments: ['dev'],
});

export const enableWorkforceInProgressRepairs = createFeatureFlag({
  description:
    'Displays the Currently Claimed DUTs / In-Progress Repairs metric and table column in ChromeOS Workforce Activity.',
  namespace: 'fleet-console',
  name: 'workforce-in-progress-repairs',
  percentage: {
    dev: 0,
    prod: 0,
  },
  allowedEnvironments: ['dev'],
});

export const enableWorkforceTimeframeFilter = createFeatureFlag({
  description:
    'Displays the timeframe picker (1D/7D/30D/YTD) in ChromeOS Workforce Activity.',
  namespace: 'fleet-console',
  name: 'workforce-timeframe-filter',
  percentage: {
    dev: 0,
    prod: 0,
  },
  allowedEnvironments: ['dev'],
});

export const enableUnifiedTaskHistory = createFeatureFlag({
  description:
    'Enables unified task history (Swarming + Mobile Harness) via the Fleet Console backend RPC for supported pilot devices.',
  namespace: 'fleet-console',
  name: 'unified-task-history',
  percentage: {
    dev: 100,
    prod: 0,
  },
  trackingBug: '542600108',
  allowedEnvironments: ['dev', 'prod'],
});

export const enablePixelDeviceRestoration = createFeatureFlag({
  description: 'Enables creating restoration bugs for Pixel devices.',
  namespace: 'fleet-console',
  name: 'pixel-device-restoration',
  percentage: {
    dev: 100,
    prod: 100,
  },
  trackingBug: '564815043',
  allowedEnvironments: ['dev', 'prod'],
});
