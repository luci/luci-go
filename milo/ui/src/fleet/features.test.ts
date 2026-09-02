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
  getFeatureFlagValue,
  isFlagAvailableInEnvironment,
} from '@/common/feature_flags';
import {
  enableAndroidUtilizationMetrics,
  enableChromeOsRepairsDashboard,
  enablePTE,
} from '@/fleet/features';

describe('Fleet Console Feature Flags Environment Isolation', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  describe('Dev Environment (luci-milo-dev / localhost)', () => {
    const env = 'dev';

    it('enables chromeos-repairs-dashboard by default in dev', () => {
      expect(
        isFlagAvailableInEnvironment(enableChromeOsRepairsDashboard, env),
      ).toBe(true);
      expect(
        getFeatureFlagValue(
          enableChromeOsRepairsDashboard,
          'user@google.com',
          env,
        ),
      ).toBe(true);
    });

    it('keeps pte-support disabled by default in dev (0% rollout until explicitly toggled)', () => {
      expect(isFlagAvailableInEnvironment(enablePTE, env)).toBe(true);
      expect(getFeatureFlagValue(enablePTE, 'user@google.com', env)).toBe(
        false,
      );

      localStorage.setItem('featureFlag:fleet-console:pte-support', 'on');
      expect(getFeatureFlagValue(enablePTE, 'user@google.com', env)).toBe(true);
    });

    it('enables android-utilization-metrics by default in dev', () => {
      expect(
        isFlagAvailableInEnvironment(enableAndroidUtilizationMetrics, env),
      ).toBe(true);
      expect(
        getFeatureFlagValue(
          enableAndroidUtilizationMetrics,
          'user@google.com',
          env,
        ),
      ).toBe(true);
    });

    it('allows toggling off flags via localStorage in dev', () => {
      localStorage.setItem(
        'featureFlag:fleet-console:chromeos-repairs-dashboard',
        'off',
      );
      expect(
        getFeatureFlagValue(
          enableChromeOsRepairsDashboard,
          'user@google.com',
          env,
        ),
      ).toBe(false);
    });
  });

  describe('Prod Environment Safety Guardrails (luci-milo.appspot.com)', () => {
    const env = 'prod';

    it('prevents chromeos-repairs-dashboard from being available or enabled in prod', () => {
      expect(
        isFlagAvailableInEnvironment(enableChromeOsRepairsDashboard, env),
      ).toBe(false);
      expect(
        getFeatureFlagValue(
          enableChromeOsRepairsDashboard,
          'user@google.com',
          env,
        ),
      ).toBe(false);
    });

    it('prevents pte-support from being available or enabled in prod', () => {
      expect(isFlagAvailableInEnvironment(enablePTE, env)).toBe(false);
      expect(getFeatureFlagValue(enablePTE, 'user@google.com', env)).toBe(
        false,
      );
    });

    it('disables android-utilization-metrics by default in prod', () => {
      expect(
        isFlagAvailableInEnvironment(enableAndroidUtilizationMetrics, env),
      ).toBe(true);
      expect(
        getFeatureFlagValue(
          enableAndroidUtilizationMetrics,
          'user@google.com',
          env,
        ),
      ).toBe(false);
    });
  });
});
