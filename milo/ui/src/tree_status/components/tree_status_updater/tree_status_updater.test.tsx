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

import { act, fireEvent, render, screen } from '@testing-library/react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TreeStatusUpdater } from './tree_status_updater';

describe('TreeStatusUpdater', () => {
  it('updates status selection based on message text', async () => {
    render(
      <FakeContextProvider>
        <TreeStatusUpdater tree="chromium" />
      </FakeContextProvider>,
    );

    const messageInput = screen.getByLabelText('Message');
    const statusSelect = screen.getByRole('combobox');

    // Default is Open
    expect(statusSelect).toHaveTextContent('Open');

    // Type "close"
    fireEvent.change(messageInput, { target: { value: 'close the tree' } });
    expect(statusSelect).toHaveTextContent('Closed');

    // Clear "close"
    fireEvent.change(messageInput, { target: { value: 'tree is ready' } });
    expect(statusSelect).toHaveTextContent('Open');

    // Type "close" and "maint"
    fireEvent.change(messageInput, {
      target: { value: 'tree is closed for maint' },
    });
    expect(statusSelect).toHaveTextContent('Maintenance');

    // Type "throt"
    fireEvent.change(messageInput, { target: { value: 'tree is throt' } });
    expect(statusSelect).toHaveTextContent('Throttled');
  });

  it('does not auto-update if status was manually changed', async () => {
    render(
      <FakeContextProvider>
        <TreeStatusUpdater tree="chromium" />
      </FakeContextProvider>,
    );

    const messageInput = screen.getByLabelText('Message');
    const statusSelectInput = screen.getByRole('combobox');

    // Manually change status to Throttled
    fireEvent.mouseDown(statusSelectInput);
    screen.getByRole('listbox');
    const throttledOption = screen.getByText('Throttled', { selector: 'li' });

    act(() => {
      fireEvent.click(throttledOption);
    });

    expect(statusSelectInput).toHaveTextContent('Throttled');

    // Type "close"
    fireEvent.change(messageInput, { target: { value: 'tree is closed' } });

    // Should still be Throttled
    expect(statusSelectInput).toHaveTextContent('Throttled');
  });
});
