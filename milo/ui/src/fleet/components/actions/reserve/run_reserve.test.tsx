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
import { ScheduleReserveRequest_ReserveFlag } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
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
  const mockFetchPermissions = jest.fn();
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

    mockFetchPermissions.mockReset();
    mockFetchPermissions.mockResolvedValue({
      hasPermission: true,
    });
    (useAdminTaskPermission as jest.Mock).mockReturnValue({
      hasPermission: true,
      fetchPermissions: mockFetchPermissions,
    });
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
    (useAdminTaskPermission as jest.Mock).mockReturnValue({
      hasPermission: null,
      fetchPermissions: mockFetchPermissions,
    });

    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Reserve' });
    expect(button).toBeDisabled();
  });

  it('opens AdminAccessRequiredDialog when user lacks permissions', async () => {
    (useAdminTaskPermission as jest.Mock).mockReturnValue({
      hasPermission: false,
      fetchPermissions: mockFetchPermissions,
    });
    mockFetchPermissions.mockResolvedValue({
      hasPermission: false,
    });

    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Reserve' });
    expect(button).toBeEnabled();
    fireEvent.click(button);

    // Verify Admin Access dialog is shown
    await waitFor(() => {
      expect(screen.getByText('Admin Access Required')).toBeVisible();
    });
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
    await waitFor(() => {
      expect(
        screen.getByText(
          'Please confirm that you want to reserve the following device:',
        ),
      ).toBeVisible();
    });
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
    const input = await screen.findByLabelText(/Comment/i);
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

    expect(mockFetchPermissions).toHaveBeenCalledTimes(1);
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
    const input = await screen.findByLabelText(/Comment/i);
    fireEvent.change(input, {
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

    expect(mockFetchPermissions).toHaveBeenCalledTimes(1);
  });

  it('calls mockFetchPermissions when reserve button is clicked', async () => {
    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Reserve' });
    fireEvent.click(button);

    await waitFor(() => {
      expect(mockFetchPermissions).toHaveBeenCalled();
    });
  });

  it('shows error snackbar when permission check fails with query error', async () => {
    (useAdminTaskPermission as jest.Mock).mockReturnValue({
      hasPermission: true,
      fetchPermissions: mockFetchPermissions,
    });
    mockFetchPermissions.mockRejectedValue(
      new Error('Permission service connection reset'),
    );

    render(
      <FakeContextProvider>
        <RunReserve selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Reserve' });
    fireEvent.click(button);

    // Verify error Snackbar is shown
    await waitFor(() => {
      expect(
        screen.getByText('Permission service connection reset'),
      ).toBeVisible();
    });
    // Reserve Dialog confirmation screen should not be open
    expect(
      screen.queryByText(/Please confirm that you want to reserve/),
    ).toBeNull();
  });

  it('triggers ScheduleReserve RPC with LATEST flag when latest is checked', async () => {
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
    const input = await screen.findByLabelText(/Comment/i);
    fireEvent.change(input, { target: { value: 'My custom reason' } });

    // Check "Use latest reserve version"
    const latestCheckbox = screen.getByRole('checkbox', {
      name: 'Use latest reserve version',
    });
    fireEvent.click(latestCheckbox);

    // Verify equivalent shivas command contains -latest
    expect(
      screen.getByText(
        /shivas reserve-duts -comment "My custom reason" -latest device-1/,
      ),
    ).toBeVisible();

    // Click Confirm
    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    fireEvent.click(confirmButton);

    // Verify ScheduleReserve called with LATEST flag
    await waitFor(() => {
      expect(mockScheduleReserve).toHaveBeenCalledWith({
        unitNames: ['device-1'],
        comment: 'My custom reason',
        flags: [ScheduleReserveRequest_ReserveFlag.LATEST],
      });
    });
  });
});
