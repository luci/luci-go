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

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useFeatureFlag } from './context';

interface TestComponentProps {
  percentage: number;
}

function TestComponent({ percentage }: TestComponentProps) {
  const flag = useFeatureFlag({
    description: 'Test flag',
    namespace: 'flagKey',
    name: 'test-flag',
    percentage,
    trackingBug: '123455',
  });
  return <>{flag ? 'flag is on' : 'flag is off'}</>;
}

describe('Feature flags', () => {
  afterEach(() => {
    localStorage.clear();
  });
  it('should enable feature when above threshold', () => {
    render(
      <FakeContextProvider>
        <TestComponent percentage={80} />
      </FakeContextProvider>,
    );
    expect(screen.getByText('flag is on')).toBeInTheDocument();
  });

  it('should disable feature when below threshold', () => {
    render(
      <FakeContextProvider>
        <TestComponent percentage={10} />
      </FakeContextProvider>,
    );
    expect(screen.getByText('flag is off')).toBeInTheDocument();
  });

  it('should adhere to localStorage overrides when flag is on', () => {
    localStorage.setItem('featureFlag:flagKey:test-flag', 'on');
    render(
      <FakeContextProvider>
        <TestComponent percentage={0} />
      </FakeContextProvider>,
    );
    waitFor(() => {
      expect(screen.getByText('flag is on')).toBeInTheDocument();
    });
  });
});
