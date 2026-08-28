// Copyright 2024 The LUCI Authors.
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

import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useMemo, useState } from 'react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  createFeatureFlag,
  FeatureFlag,
  getEnabledFeatureFlags,
  getFeatureFlagKey,
  getFeatureFlagLocalStorageKey,
  getFeatureFlagValue,
  getFlagRolloutPercentage,
  isFlagAvailableInEnvironment,
  REGISTERED_FLAGS,
  resetRegisteredFlagsForTesting,
  RolloutPercentage,
  useAvailableFlags,
  useFeatureFlag,
} from './context';

function createSampleFlag(percentage: RolloutPercentage) {
  return createFeatureFlag({
    description: 'Test flag',
    namespace: 'flagKey',
    name: 'test-flag',
    percentage,
    trackingBug: '123455',
  });
}

interface TestComponentProps {
  flag: FeatureFlag;
}
function TestComponent({ flag }: TestComponentProps) {
  const flagStatus = useFeatureFlag(flag);
  return <>{flagStatus ? 'flag is on' : 'flag is off'}</>;
}

interface TestMultipleFlagsComponentProps {
  count: number;
}

function TestMultipleFlagsComponent({
  count,
}: TestMultipleFlagsComponentProps) {
  const availableFlags = useAvailableFlags();
  const [componentCount, setComponentCount] = useState(count);
  function decreaseComponentCount() {
    setComponentCount((count) => count - 1);
  }
  const flag = useMemo(() => createSampleFlag(10), []);
  return (
    <>
      {Array.from({ length: componentCount }).map((_, index) => (
        <TestComponent key={index} flag={flag} />
      ))}
      <button onClick={() => decreaseComponentCount()}>decrease count</button>
      <p>available flags {availableFlags.get(flag)?.observers.size}</p>
      <p>available observers {availableFlags.get(flag)?.observers.size}</p>
    </>
  );
}

describe('Feature flags', () => {
  beforeEach(() => {
    resetRegisteredFlagsForTesting();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('should enable feature when above threshold', () => {
    const flag = createSampleFlag(80);
    render(
      <FakeContextProvider>
        <TestComponent flag={flag} />
      </FakeContextProvider>,
    );
    expect(screen.getByText('flag is on')).toBeInTheDocument();
  });

  it('should disable feature when below threshold', () => {
    const flag = createSampleFlag(10);

    render(
      <FakeContextProvider>
        <TestComponent flag={flag} />
      </FakeContextProvider>,
    );
    expect(screen.getByText('flag is off')).toBeInTheDocument();
  });

  it('should adhere to localStorage overrides when flag is on', async () => {
    const flag = createSampleFlag(0);
    localStorage.setItem('featureFlag:flagKey:test-flag', 'on');
    render(
      <FakeContextProvider>
        <TestComponent flag={flag} />
      </FakeContextProvider>,
    );
    await waitFor(() => {
      expect(screen.getByText('flag is on')).toBeInTheDocument();
    });
  });

  it('should increment number of flags when another component is used', async () => {
    render(
      <FakeContextProvider>
        <TestMultipleFlagsComponent count={2} />
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('available flags 2')).toBeInTheDocument();
      expect(screen.getByText('available observers 2')).toBeInTheDocument();
    });

    await userEvent.click(screen.getByText('decrease count'));
    await waitFor(() => {
      expect(screen.getByText('available flags 1')).toBeInTheDocument();
      expect(screen.getByText('available observers 1')).toBeInTheDocument();
    });
  });

  it('should determine environment availability based on allowedEnvironments and fallback to dev and prod', () => {
    const devOnlyFlag = createFeatureFlag({
      description: 'Dev flag',
      namespace: 'test',
      name: 'dev-flag',
      percentage: 0,
      trackingBug: '123',
      allowedEnvironments: ['dev'],
    });
    const defaultFlag = createFeatureFlag({
      description: 'Default flag',
      namespace: 'test',
      name: 'default-flag',
      percentage: 0,
      trackingBug: '123',
    });

    expect(isFlagAvailableInEnvironment(devOnlyFlag, 'dev')).toBe(true);
    expect(isFlagAvailableInEnvironment(devOnlyFlag, 'prod')).toBe(false);
    expect(isFlagAvailableInEnvironment(defaultFlag, 'dev')).toBe(true);
    expect(isFlagAvailableInEnvironment(defaultFlag, 'prod')).toBe(true);
  });

  it('should support environment-specific rollout percentages e.g. dev: 100, prod: 0', () => {
    const flag = createFeatureFlag({
      description: 'Env rollout flag',
      namespace: 'test',
      name: 'env-rollout',
      percentage: { dev: 100, prod: 0 },
      trackingBug: '12345',
    });

    expect(getFlagRolloutPercentage(flag.config, 'dev')).toBe(100);
    expect(getFlagRolloutPercentage(flag.config, 'prod')).toBe(0);

    expect(getFeatureFlagValue(flag, 'test-user@google.com', 'dev')).toBe(true);
    expect(getFeatureFlagValue(flag, 'test-user@google.com', 'prod')).toBe(
      false,
    );
  });

  it('should format keys cleanly with getFeatureFlagKey and getFeatureFlagLocalStorageKey', () => {
    const flag = createSampleFlag(50);
    expect(getFeatureFlagKey(flag)).toBe('flagKey:test-flag');
    expect(getFeatureFlagLocalStorageKey(flag)).toBe(
      'featureFlag:flagKey:test-flag',
    );
  });

  it('should auto-register and deduplicate created feature flags in REGISTERED_FLAGS during HMR', () => {
    const flag1 = createSampleFlag(50);
    expect(REGISTERED_FLAGS.size).toBe(1);
    expect(REGISTERED_FLAGS.get('flagKey:test-flag')).toBe(flag1);

    // Simulate HMR re-evaluating createFeatureFlag with updated percentage
    const flag2 = createSampleFlag(100);
    expect(REGISTERED_FLAGS.size).toBe(1);
    expect(REGISTERED_FLAGS.get('flagKey:test-flag')).toBe(flag2);
  });

  it('should return list of enabled feature flags via getEnabledFeatureFlags', () => {
    createFeatureFlag({
      description: 'Active flag',
      namespace: 'ns',
      name: 'active',
      percentage: 100,
      trackingBug: '123',
    });
    createFeatureFlag({
      description: 'Inactive flag',
      namespace: 'ns',
      name: 'inactive',
      percentage: 0,
      trackingBug: '123',
    });

    const enabled = getEnabledFeatureFlags();
    expect(enabled).toContain('ns:active');
    expect(enabled).not.toContain('ns:inactive');
  });

  it('should evaluate feature flag status synchronously via getFeatureFlagValue', () => {
    const flag = createSampleFlag(0);
    expect(getFeatureFlagValue(flag)).toBe(false);

    localStorage.setItem('featureFlag:flagKey:test-flag', 'on');
    expect(getFeatureFlagValue(flag)).toBe(true);

    localStorage.setItem('featureFlag:flagKey:test-flag', 'off');
    expect(getFeatureFlagValue(flag)).toBe(false);
  });

  it('should evaluate anonymous identity correctly for 100% vs partial rollouts', () => {
    const flag100 = createSampleFlag(100);
    const flag50 = createSampleFlag(50);

    expect(getFeatureFlagValue(flag100)).toBe(true);
    expect(getFeatureFlagValue(flag50)).toBe(false);
  });

  it('should safely handle localStorage access exceptions', () => {
    const flag = createSampleFlag(100);
    jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('SecurityError', 'SecurityError');
    });

    expect(() => getFeatureFlagValue(flag)).not.toThrow();
    expect(getFeatureFlagValue(flag)).toBe(true);
    expect(() => getEnabledFeatureFlags()).not.toThrow();

    jest.restoreAllMocks();
  });
});
