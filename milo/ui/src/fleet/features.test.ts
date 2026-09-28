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
  createFeatureFlag,
  type FeatureFlag,
  getFeatureFlagLocalStorageKey,
  getFeatureFlagValue,
  getFlagRolloutPercentage,
  isFlagAvailableInEnvironment,
  resetRegisteredFlagsForTesting,
} from '@/common/feature_flags';
import * as fleetFeatures from '@/fleet/features';

describe('Fleet Console Feature Flags Framework & Contract Tests', () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    resetRegisteredFlagsForTesting();
  });

  describe('Environment-Scoped Rollout & Availability', () => {
    it('evaluates dev-only flags as enabled in dev and blocked in prod', () => {
      const devOnlyFlag = createFeatureFlag({
        description: 'Dev-only test flag.',
        namespace: 'fleet-console',
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
      const stagedFlag = createFeatureFlag({
        description: 'Staged rollout flag (100% dev, 0% prod).',
        namespace: 'fleet-console',
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
      const launchedFlag = createFeatureFlag({
        description: 'Fully launched flag.',
        namespace: 'fleet-console',
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
  });

  describe('localStorage Developer Overrides', () => {
    it('respects on and off localStorage overrides in allowed environments', () => {
      const flag = createFeatureFlag({
        description: 'Overrideable flag.',
        namespace: 'fleet-console',
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
      const devOnlyFlag = createFeatureFlag({
        description: 'Strict dev-only flag.',
        namespace: 'fleet-console',
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
  });

  describe('@/fleet/features Exported Flag Contracts', () => {
    it('validates all exported Fleet Console feature flags meet schema and environment invariants', () => {
      const exportedFlags = Object.values(fleetFeatures).filter(
        (val): val is FeatureFlag =>
          Boolean(val && typeof val === 'object' && 'config' in val),
      );

      expect(exportedFlags.length).toBeGreaterThan(0);

      for (const flag of exportedFlags) {
        expect(flag.config.namespace).toBe('fleet-console');
        expect(flag.config.name.trim().length).toBeGreaterThan(0);
        expect(flag.config.description.trim().length).toBeGreaterThan(0);

        const devPct = getFlagRolloutPercentage(flag.config, 'dev');
        const prodPct = getFlagRolloutPercentage(flag.config, 'prod');
        expect(devPct).toBeGreaterThanOrEqual(0);
        expect(devPct).toBeLessThanOrEqual(100);
        expect(prodPct).toBeGreaterThanOrEqual(0);
        expect(prodPct).toBeLessThanOrEqual(100);
      }
    });
  });
});
