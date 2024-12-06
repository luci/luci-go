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
  useAvailableFlags,
  useFeatureFlag,
} from './context';

function createSampleFlag(percentage: number) {
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
});
