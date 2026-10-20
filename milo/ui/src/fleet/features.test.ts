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

import { FeatureFlag } from '@/common/feature_flags';

import * as fleetFeatures from './features';

describe('createFleetFeatureFlag', () => {
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
