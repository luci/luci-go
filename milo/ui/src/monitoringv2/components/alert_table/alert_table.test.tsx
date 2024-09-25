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

import { fireEvent, render, screen } from '@testing-library/react';
import { act } from 'react';

import { configuredTrees } from '@/monitoringv2/util/config';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { testAlert, testAlert2 } from '../testing_tools/test_utils';

import { AlertTable } from './alert_table';

describe('<AlertTable />', () => {
  it('displays an alert', async () => {
    render(
      <FakeContextProvider>
        <AlertTable tree={configuredTrees[0]} alerts={[testAlert]} bugs={[]} />
      </FakeContextProvider>,
    );
    expect(screen.getByText('linux-rel')).toBeInTheDocument();
    expect(screen.getByText('compile')).toBeInTheDocument();
  });

  it('expands an alert on click', async () => {
    render(
      <FakeContextProvider>
        <AlertTable tree={configuredTrees[0]} alerts={[testAlert]} bugs={[]} />
      </FakeContextProvider>,
    );
    expect(screen.getByText('compile')).toBeInTheDocument();
    expect(screen.queryByText('test.Example')).toBeNull();
    await act(() => fireEvent.click(screen.getByText('compile')));
    expect(screen.getByText('test.Example')).toBeInTheDocument();
  });

  it('sorts alerts on header click', async () => {
    render(
      <FakeContextProvider>
        <AlertTable
          tree={configuredTrees[0]}
          alerts={[testAlert, testAlert2]}
          bugs={[]}
        />
      </FakeContextProvider>,
    );
    expect(screen.getByText('linux-rel')).toBeInTheDocument();
    expect(screen.getByText('win-rel')).toBeInTheDocument();
    // Sort asc
    await act(() => fireEvent.click(screen.getByText('Failed Builder')));
    let linux = screen.getByText('linux-rel');
    let win = screen.getByText('win-rel');
    expect(linux.compareDocumentPosition(win)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    // Sort desc
    await act(() => fireEvent.click(screen.getByText('Failed Builder')));
    linux = screen.getByText('linux-rel');
    win = screen.getByText('win-rel');
    expect(linux.compareDocumentPosition(win)).toBe(
      Node.DOCUMENT_POSITION_PRECEDING,
    );
  });
});
