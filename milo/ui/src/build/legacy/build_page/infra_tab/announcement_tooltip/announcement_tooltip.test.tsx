// Copyright 2023 The LUCI Authors.
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

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { act } from 'react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { InfraTabAnnouncementTooltip } from './announcement_tooltip';

describe('<InfraTabAnnouncementTooltip />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    localStorage.clear();
    cleanup();
  });

  it('can dismiss release notes', async () => {
    render(
      <FakeContextProvider>
        <InfraTabAnnouncementTooltip>
          <div />
        </InfraTabAnnouncementTooltip>
      </FakeContextProvider>,
    );
    expect(
      screen.getByText('have been moved to the infra tab', { exact: false }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByText('Dismiss'));
    await act(() => jest.advanceTimersByTimeAsync(1000));
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(
      screen.queryByText('have been moved to the infra tab', { exact: false }),
    ).not.toBeInTheDocument();
  });

  it('can dismiss release notes by clicking away', async () => {
    render(
      <FakeContextProvider>
        <InfraTabAnnouncementTooltip>
          <div />
        </InfraTabAnnouncementTooltip>
      </FakeContextProvider>,
    );
    expect(
      screen.getByText('have been moved to the infra tab', { exact: false }),
    ).toBeInTheDocument();

    // Persist the tooltip for a while initially.
    act(() => fireEvent.mouseDown(document.body));
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(
      screen.queryByText('have been moved to the infra tab', { exact: false }),
    ).toBeInTheDocument();

    // Still persist after 5s when there are no more clicks.
    await act(() => jest.advanceTimersByTimeAsync(5000));
    expect(
      screen.queryByText('have been moved to the infra tab', { exact: false }),
    ).toBeInTheDocument();

    // Dismiss when the user clicks away after 5s.
    act(() => fireEvent.mouseDown(document.body));
    await act(() => jest.advanceTimersByTimeAsync(1000));
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(
      screen.queryByText('have been moved to the infra tab', { exact: false }),
    ).not.toBeInTheDocument();
  });
});
