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
import { useState } from 'react';

import { createFeatureFlag, useFeatureFlag } from '@/common/feature_flags';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AvailableFlags } from './available_flags';

const flag = createFeatureFlag({
  description: 'Test flag',
  namespace: 'flagKey',
  name: 'test-flag',
  percentage: 0,
  trackingBug: '123455',
});

function TestComponent() {
  const flagStatus = useFeatureFlag(flag);
  return <div>{flagStatus ? 'flag is on' : 'flag is off'}</div>;
}

function TestRemoveFirstComponent() {
  const [viewFirst, setViewFirst] = useState(true);

  function handleHideFirstClicked() {
    setViewFirst(false);
  }

  return (
    <>
      {viewFirst && <TestComponent />}
      <TestComponent />
      <button onClick={() => handleHideFirstClicked()}>Hide first</button>
    </>
  );
}

describe('<AvailableFlags />', () => {
  afterEach(() => {
    localStorage.clear();
  });

  it('should display available flags in the page', async () => {
    render(
      <FakeContextProvider>
        <AvailableFlags />
        <TestComponent />
      </FakeContextProvider>,
    );
    expect(screen.getByText('flag is off')).toBeVisible();
    await userEvent.click(screen.getByTitle('Toggle feature flags'));
    expect(screen.getByText('Feature flags')).toBeVisible();
    expect(screen.getByText('flagKey:test-flag')).toBeVisible();
    expect(screen.getByText('Test flag')).toBeVisible();
  });

  it('should switch flag status when toggle clicked', async () => {
    render(
      <FakeContextProvider>
        <AvailableFlags />
        <TestComponent />
        <TestComponent />
      </FakeContextProvider>,
    );
    expect(screen.getAllByText('flag is off')).toHaveLength(2);
    await userEvent.click(screen.getByTitle('Toggle feature flags'));
    await userEvent.click(screen.getByRole('checkbox'));
    await waitFor(() => {
      expect(screen.getAllByText('flag is on')).toHaveLength(2);
    });
  });

  it('should still override even if first component is removed', async () => {
    render(
      <FakeContextProvider>
        <AvailableFlags />
        <TestRemoveFirstComponent />
      </FakeContextProvider>,
    );
    expect(screen.getAllByText('flag is off')).toHaveLength(2);
    await userEvent.click(screen.getByText('Hide first'));
    await userEvent.click(screen.getByTitle('Toggle feature flags'));
    await userEvent.click(screen.getByRole('checkbox'));
    await waitFor(() => {
      expect(screen.getAllByText('flag is on')).toHaveLength(1);
    });
  });
});
