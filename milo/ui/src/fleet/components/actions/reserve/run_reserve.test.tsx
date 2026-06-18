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

import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useAdminTaskPermission } from '../shared/use_admin_task_permission';

import { RunReserve } from './run_reserve';

// Mock the hooks
jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useFleetConsoleClient: jest.fn(),
}));

jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: jest.fn(),
}));

jest.mock('../shared/use_admin_task_permission', () => ({
  useAdminTaskPermission: jest.fn(),
}));

describe('<RunReserve />', () => {
  const mockScheduleReserve = jest.fn();
  const mockTrackEvent = jest.fn();
  const selectedDuts = [
    { name: 'device-1', dutId: 'device-1-id', namespace: 'os' },
  ];

  beforeEach(() => {
    jest.clearAllMocks();

    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      ScheduleReserve: mockScheduleReserve,
    });

    (useGoogleAnalytics as jest.Mock).mockReturnValue({
      trackEvent: mockTrackEvent,
    });

    (useAdminTaskPermission as jest.Mock).mockReturnValue(true);
  });

  it('renders button, disabled when no DUTs selected', async () => {
    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={[]} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Reserve' });
    expect(button).toBeVisible();
    expect(button).toBeDisabled();
  });

  it('disables button when permissions are loading', async () => {
    (useAdminTaskPermission as jest.Mock).mockReturnValue(null);

    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Reserve' });
    expect(button).toBeDisabled();
  });

  it('opens AdminAccessRequiredDialog when user lacks permissions', async () => {
    (useAdminTaskPermission as jest.Mock).mockReturnValue(false);

    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Reserve' });
    expect(button).toBeEnabled();
    fireEvent.click(button);

    // Verify Admin Access dialog is shown
    expect(screen.getByText('Admin Access Required')).toBeVisible();
    expect(screen.getByText('fleet-console-admin-tasks-policy')).toBeVisible();
    // Reserve Dialog confirmation screen should not be open
    expect(
      screen.queryByText(/Please confirm that you want to reserve/),
    ).toBeNull();
  });

  it('opens ReserveDialog when user has permissions and clicks Reserve', async () => {
    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Reserve' });
    expect(button).toBeEnabled();
    fireEvent.click(button);

    // Reserve Dialog confirmation screen should open
    expect(
      screen.getByText(
        'Please confirm that you want to reserve the following device:',
      ),
    ).toBeVisible();
    expect(screen.getByRole('link', { name: 'device-1' })).toBeVisible();
  });

  it('triggers ScheduleReserve RPC, tracks GA, and shows results on success', async () => {
    mockScheduleReserve.mockResolvedValue({
      sessionId: 'test-session-id',
      results: [
        {
          unitName: 'device-1',
          taskUrl: 'https://milo/task-1',
        },
      ],
    });

    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    // Open Dialog
    fireEvent.click(screen.getByRole('button', { name: 'Reserve' }));

    // Type a comment
    const input = screen.getByLabelText(/Comment/i);
    fireEvent.change(input, { target: { value: 'My custom reason' } });

    // Click Confirm
    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    fireEvent.click(confirmButton);

    // Verify GA event tracked
    expect(mockTrackEvent).toHaveBeenCalledWith('run_reserve', {
      componentName: 'run_reserve_button',
      dutCount: 1,
    });

    // Wait for the API to resolve and show results screen
    await waitFor(() => {
      expect(
        screen.getByText('Reserve has been triggered on the following device:'),
      ).toBeVisible();
    });

    expect(screen.getByRole('link', { name: 'View in Milo' })).toHaveAttribute(
      'href',
      'https://milo/task-1',
    );

    expect(
      screen.getByRole('link', { name: 'View tasks in Swarming' }),
    ).toHaveAttribute('href', expect.stringContaining('test-session-id'));
  });

  it('handles API failure correctly and displays error message', async () => {
    mockScheduleReserve.mockRejectedValue(
      new Error('Internal permission error'),
    );

    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    // Open and submit Dialog
    fireEvent.click(screen.getByRole('button', { name: 'Reserve' }));
    fireEvent.change(screen.getByLabelText(/Comment/i), {
      target: { value: 'My reason' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    // Wait for error results screen
    await waitFor(() => {
      expect(
        screen.getByText(
          'Failed to schedule reserve: Internal permission error',
        ),
      ).toBeVisible();
    });
  });
});
