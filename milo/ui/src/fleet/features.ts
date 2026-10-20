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

import { useEffect } from 'react';
import { useLocation } from 'react-router';

import {
  createFeatureFlag,
  getFeatureFlagKey,
  getFeatureFlagLocalStorageKey,
  useFeatureFlag,
  type FeatureEnvironment,
  type FeatureFlag,
  type FeatureFlagConfig,
} from '@/common/feature_flags';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

export interface FleetFeatureFlagConfig
  extends Omit<FeatureFlagConfig, 'namespace' | 'allowedEnvironments'> {
  readonly namespace?: 'fleet-console';
  readonly allowedEnvironments: readonly [
    FeatureEnvironment,
    ...FeatureEnvironment[],
  ];
}

/**
 * Registers a Fleet Console feature flag with a compile-time requirement for
 * `allowedEnvironments` (`['dev']` or `['dev', 'prod']`).
 */
export function createFleetFeatureFlag(
  config: FleetFeatureFlagConfig,
): FeatureFlag {
  return createFeatureFlag({
    ...config,
    namespace: config.namespace ?? 'fleet-console',
  });
}

export const enableAndroidUtilizationMetrics = createFleetFeatureFlag({
  description:
    'Displays average Android device utilization metrics, columns, and filters.',
  name: 'android-utilization-metrics',
  percentage: {
    dev: 100,
    prod: 0,
  },
  trackingBug: '525089400',
  allowedEnvironments: ['dev', 'prod'],
});

export const enablePTE = createFleetFeatureFlag({
  description: 'Enables PTE support in the fleet console.',
  name: 'pte-support',
  percentage: 100,
  trackingBug: '503760268',
  allowedEnvironments: ['dev', 'prod'],
});

export const enableChromeOsRepairsDashboard = createFleetFeatureFlag({
  description: 'Displays the ChromeOS Repair Dashboard shell.',
  name: 'chromeos-repairs-dashboard',
  percentage: 100,
  trackingBug: '542600108',
  allowedEnvironments: ['dev', 'prod'],
});

export const enableAndroidHealthMetrics = createFleetFeatureFlag({
  description:
    'Displays unified Android device health metrics based on health categories.',
  name: 'android-health-metrics',
  percentage: 100,
  trackingBug: '537412303',
  allowedEnvironments: ['dev', 'prod'],
});

export const enableChromeOsWorkforceActivity = createFleetFeatureFlag({
  description:
    'Displays the ChromeOS Workforce Activity monitoring view in repairs.',
  name: 'chromeos-workforce-activity',
  percentage: 100,
  trackingBug: '565724356',
  allowedEnvironments: ['dev', 'prod'],
});

export const enableWorkforcePickupRank = createFleetFeatureFlag({
  description:
    'Displays the Avg Queue Pickup Rank metric and table column in ChromeOS Workforce Activity.',
  name: 'workforce-avg-queue-pickup-rank',
  percentage: {
    dev: 0,
    prod: 0,
  },
  allowedEnvironments: ['dev'],
});

export const enableWorkforceInProgressRepairs = createFleetFeatureFlag({
  description:
    'Displays the Currently Claimed DUTs / In-Progress Repairs metric and table column in ChromeOS Workforce Activity.',
  name: 'workforce-in-progress-repairs',
  percentage: {
    dev: 0,
    prod: 0,
  },
  allowedEnvironments: ['dev'],
});

export const enableWorkforceTimeframeFilter = createFleetFeatureFlag({
  description:
    'Displays the timeframe picker (1D/7D/30D/YTD) in ChromeOS Workforce Activity.',
  name: 'workforce-timeframe-filter',
  percentage: {
    dev: 0,
    prod: 0,
  },
  allowedEnvironments: ['dev'],
});

export const enableUnifiedTaskHistory = createFleetFeatureFlag({
  description:
    'Enables unified task history (Swarming + Mobile Harness) via the Fleet Console backend RPC for supported pilot devices.',
  name: 'unified-task-history',
  percentage: {
    dev: 100,
    prod: 0,
  },
  trackingBug: '542600108',
  allowedEnvironments: ['dev', 'prod'],
});

export const enablePixelDeviceRestoration = createFleetFeatureFlag({
  description: 'Enables creating restoration bugs for Pixel devices.',
  name: 'pixel-device-restoration',
  percentage: {
    dev: 100,
    prod: 100,
  },
  trackingBug: '564815043',
  allowedEnvironments: ['dev', 'prod'],
});

export const enableAndroidRepairQueue = createFleetFeatureFlag({
  description: 'Enable the manual repair queue dashboard for Android and Pixel',
  name: 'android-repair-queue',
  percentage: { dev: 100, prod: 0 },
  trackingBug: '568290011',
  allowedEnvironments: ['dev'],
});

export const enableProductCatalogRevamp = createFleetFeatureFlag({
  description:
    'Enables the revamped Product Catalog card view, hardware reference photos, and unified details view.',
  name: 'product-catalog-revamp',
  percentage: {
    dev: 100,
    prod: 0,
  },
  trackingBug: '571288184',
  allowedEnvironments: ['dev', 'prod'],
});

function parseRevampUrlOverride(search: string): 'on' | 'off' | null {
  const params = new URLSearchParams(search);
  const fullKey = getFeatureFlagKey(enableProductCatalogRevamp);
  const shortName = enableProductCatalogRevamp.config.name;
  const direct = params.get(`ff:${fullKey}`) || params.get(`ff:${shortName}`);
  if (direct) {
    const lower = direct.toLowerCase();
    if (lower === 'on' || lower === 'true' || lower === '1') return 'on';
    if (lower === 'off' || lower === 'false' || lower === '0') return 'off';
  }
  for (const val of params.getAll('ff')) {
    const parts = val.split(':');
    const isMatch =
      (parts.length === 2 &&
        (parts[0] === fullKey || parts[0] === shortName)) ||
      (parts.length === 3 && `${parts[0]}:${parts[1]}` === fullKey);
    if (isMatch) {
      const lower = parts[parts.length - 1].toLowerCase();
      if (lower === 'on' || lower === 'true' || lower === '1') return 'on';
      if (lower === 'off' || lower === 'false' || lower === '0') return 'off';
    }
  }
  return null;
}

/**
 * Evaluates `enableProductCatalogRevamp` while migrating any URL/session `?ff=`
 * override into `localStorage` so the top-bar Feature Flags toggle switch can
 * freely enable and disable the revamp during a session.
 */
export function useProductCatalogRevamp(): boolean {
  const { search } = useLocation();
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  // Synchronously migrate any URL or sessionStorage `?ff=` override into
  // `localStorage` BEFORE calling `useFeatureFlag` so `useLocalStorage` reads
  // the override on the initial render and the top-bar Feature Flags drawer
  // switch can freely toggle the revamp on and off during the session.
  const fullKey = getFeatureFlagKey(enableProductCatalogRevamp);
  const shortName = enableProductCatalogRevamp.config.name;
  const localKey = getFeatureFlagLocalStorageKey(enableProductCatalogRevamp);

  if (typeof window !== 'undefined') {
    const urlOrSessionOverride =
      parseRevampUrlOverride(window.location.search) ||
      (window.sessionStorage.getItem(`ff:${fullKey}`) as 'on' | 'off' | null);

    if (urlOrSessionOverride === 'on' || urlOrSessionOverride === 'off') {
      try {
        window.sessionStorage.removeItem(`ff:${fullKey}`);
        window.localStorage.setItem(localKey, urlOrSessionOverride);
        const url = new URL(window.location.href);
        if (
          url.searchParams.has('ff') ||
          url.searchParams.has(`ff:${fullKey}`) ||
          url.searchParams.has(`ff:${shortName}`)
        ) {
          url.searchParams.delete('ff');
          url.searchParams.delete(`ff:${fullKey}`);
          url.searchParams.delete(`ff:${shortName}`);
          window.history.replaceState(window.history.state, '', url.toString());
        }
      } catch {
        // Ignore storage or history errors
      }
    }
  }

  const flagStatus = useFeatureFlag(enableProductCatalogRevamp);

  // Also strip `ff` from `SyncedSearchParamsContext` in an effect when a
  // localStorage override is active so filter changes on the Product Catalog
  // page do not write `ff` back into the URL.
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const rawLocal = window.localStorage.getItem(localKey);
    if (rawLocal !== 'on' && rawLocal !== 'off') return;
    if (
      searchParams.has('ff') ||
      searchParams.has(`ff:${fullKey}`) ||
      searchParams.has(`ff:${shortName}`)
    ) {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.delete('ff');
          next.delete(`ff:${fullKey}`);
          next.delete(`ff:${shortName}`);
          return next;
        },
        { replace: true },
      );
    }
  }, [fullKey, localKey, searchParams, setSearchParams, shortName]);

  if (typeof window !== 'undefined') {
    const rawLocal = window.localStorage.getItem(localKey);
    if (rawLocal === 'on' || rawLocal === 'off') {
      return flagStatus;
    }
    const windowOverride = parseRevampUrlOverride(window.location.search);
    if (windowOverride !== null) {
      return windowOverride === 'on';
    }
  }

  const routerOverride = parseRevampUrlOverride(search);
  if (routerOverride !== null) {
    return routerOverride === 'on';
  }

  return flagStatus;
}
