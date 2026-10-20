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
  FeatureFlag,
  getFeatureFlagLocalStorageKey,
  getFeatureFlagValue,
  isFlagAvailableInEnvironment,
  resetRegisteredFlagsForTesting,
} from '@/common/feature_flags';

import * as fleetFeatures from './features';

describe('createFleetFeatureFlag', () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    resetRegisteredFlagsForTesting();
  });

  it('defaults namespace to fleet-console and preserves allowedEnvironments', () => {
    const flag = fleetFeatures.createFleetFeatureFlag({
      name: 'test-fleet-flag',
      description: 'Test flag for Fleet Console',
      percentage: { dev: 100, prod: 0 },
      allowedEnvironments: ['dev'],
    });

    expect(flag.config.namespace).toBe('fleet-console');
    expect(flag.config.allowedEnvironments).toEqual(['dev']);
  });

  it('evaluates dev-only flags as enabled in dev and blocked in prod', () => {
    const devOnlyFlag = fleetFeatures.createFleetFeatureFlag({
      description: 'Dev-only test flag.',
      name: 'test-dev-only-flag',
      percentage: {
        dev: 100,
        prod: 0,
      },
      allowedEnvironments: ['dev'],
    });

    expect(isFlagAvailableInEnvironment(devOnlyFlag, 'dev')).toBe(true);
    expect(getFeatureFlagValue(devOnlyFlag, 'user@google.com', 'dev')).toBe(
      true,
    );

    expect(isFlagAvailableInEnvironment(devOnlyFlag, 'prod')).toBe(false);
    expect(getFeatureFlagValue(devOnlyFlag, 'user@google.com', 'prod')).toBe(
      false,
    );
  });

  it('evaluates multi-environment flags per environment rollout percentage', () => {
    const stagedFlag = fleetFeatures.createFleetFeatureFlag({
      description: 'Staged rollout flag (100% dev, 0% prod).',
      name: 'test-staged-flag',
      percentage: {
        dev: 100,
        prod: 0,
      },
      allowedEnvironments: ['dev', 'prod'],
    });

    expect(isFlagAvailableInEnvironment(stagedFlag, 'dev')).toBe(true);
    expect(getFeatureFlagValue(stagedFlag, 'user@google.com', 'dev')).toBe(
      true,
    );

    expect(isFlagAvailableInEnvironment(stagedFlag, 'prod')).toBe(true);
    expect(getFeatureFlagValue(stagedFlag, 'user@google.com', 'prod')).toBe(
      false,
    );
  });

  it('evaluates fully launched flags (100% scalar percentage) in both environments', () => {
    const launchedFlag = fleetFeatures.createFleetFeatureFlag({
      description: 'Fully launched flag.',
      name: 'test-launched-flag',
      percentage: 100,
      allowedEnvironments: ['dev', 'prod'],
    });

    expect(getFeatureFlagValue(launchedFlag, 'user@google.com', 'dev')).toBe(
      true,
    );
    expect(getFeatureFlagValue(launchedFlag, 'user@google.com', 'prod')).toBe(
      true,
    );
  });

  it('respects on and off localStorage overrides in allowed environments', () => {
    const flag = fleetFeatures.createFleetFeatureFlag({
      description: 'Overrideable flag.',
      name: 'test-override-flag',
      percentage: 0,
      allowedEnvironments: ['dev', 'prod'],
    });

    expect(getFeatureFlagValue(flag, 'user@google.com', 'dev')).toBe(false);

    localStorage.setItem(getFeatureFlagLocalStorageKey(flag), 'on');
    expect(getFeatureFlagValue(flag, 'user@google.com', 'dev')).toBe(true);

    localStorage.setItem(getFeatureFlagLocalStorageKey(flag), 'off');
    expect(getFeatureFlagValue(flag, 'user@google.com', 'dev')).toBe(false);
  });

  it('never enables a dev-only flag in prod even when localStorage is set to on', () => {
    const devOnlyFlag = fleetFeatures.createFleetFeatureFlag({
      description: 'Strict dev-only flag.',
      name: 'test-strict-dev-only',
      percentage: {
        dev: 100,
        prod: 0,
      },
      allowedEnvironments: ['dev'],
    });

    localStorage.setItem(getFeatureFlagLocalStorageKey(devOnlyFlag), 'on');
    expect(getFeatureFlagValue(devOnlyFlag, 'user@google.com', 'prod')).toBe(
      false,
    );
  });

  it('registers all exported Fleet Console feature flags with explicit allowedEnvironments', () => {
    const exportedFlags = Object.values(fleetFeatures).filter(
      (value): value is FeatureFlag =>
        typeof value === 'object' && value !== null && 'config' in value,
    );

    expect(exportedFlags.length).toBeGreaterThan(0);
    for (const flag of exportedFlags) {
      expect(flag.config.namespace).toBe('fleet-console');
      expect(flag.config.allowedEnvironments?.length).toBeGreaterThan(0);
      const pct = flag.config.percentage;
      const devPct = typeof pct === 'number' ? pct : pct.dev;
      const prodPct = typeof pct === 'number' ? pct : (pct.prod ?? 0);
      expect(devPct).toBeGreaterThanOrEqual(0);
      expect(devPct).toBeLessThanOrEqual(100);
      expect(prodPct).toBeGreaterThanOrEqual(0);
      expect(prodPct).toBeLessThanOrEqual(100);
    }
  });
});
