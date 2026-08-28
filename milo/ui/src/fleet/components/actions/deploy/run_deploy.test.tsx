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

import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useAdminTaskPermission } from '../shared/use_admin_task_permission';

import { RunDeploy } from './run_deploy';

jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: jest.fn(),
}));

jest.mock('../shared/use_admin_task_permission', () => ({
  useAdminTaskPermission: jest.fn(),
}));

describe('<RunDeploy />', () => {
  const mockFetchPermissions = jest.fn();
  const mockTrackEvent = jest.fn();
  const selectedDuts = [
    { name: 'device-1', dutId: 'device-1-id', namespace: 'os' },
  ];

  beforeEach(() => {
    FleetConsoleMockAPI.enableBrowserInterceptor();
    FleetConsoleMockAPI.resetFixtures();

    jest.clearAllMocks();
    mockFetchPermissions.mockReset();
    mockFetchPermissions.mockResolvedValue({
      hasPermission: true,
    });
    (useAdminTaskPermission as jest.Mock).mockReturnValue({
      hasPermission: true,
      fetchPermissions: mockFetchPermissions,
    });
    FleetConsoleMockAPI.setFixture('ScheduleDeploy', {});
    (useGoogleAnalytics as jest.Mock).mockReturnValue({
      trackEvent: mockTrackEvent,
    });
  });

  it('should render', async () => {
    render(
      <FakeContextProvider>
        <RunDeploy selectedDuts={[]} />
      </FakeContextProvider>,
    );

    const label = screen.getByText('Deploy');
    expect(label).toBeVisible();
  });

  it('calls mockFetchPermissions when clicked', async () => {
    render(
      <FakeContextProvider>
        <RunDeploy selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Deploy' });
    fireEvent.click(button);

    await waitFor(() => {
      expect(mockFetchPermissions).toHaveBeenCalledTimes(1);
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
        <RunDeploy selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    const button = screen.getByRole('button', { name: 'Deploy' });
    fireEvent.click(button);

    // Verify error Snackbar is shown
    await waitFor(() => {
      expect(
        screen.getByText('Permission service connection reset'),
      ).toBeVisible();
    });
    // Dialog should not open
    expect(screen.queryByText(/Please confirm that you want to/)).toBeNull();
  });

  it('handles ScheduleDeploy RPC failure cleanly and displays error message', async () => {
    FleetConsoleMockAPI.setFixture('ScheduleDeploy', null);

    render(
      <FakeContextProvider>
        <RunDeploy selectedDuts={selectedDuts} />
      </FakeContextProvider>,
    );

    // Open Dialog
    fireEvent.click(screen.getByRole('button', { name: 'Deploy' }));

    // Wait for Deploy dialog to open and click confirm
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Confirm' })).toBeVisible();
    });
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    // Wait for error results screen
    await waitFor(() => {
      expect(screen.getByText(/Failed to schedule deploy/i)).toBeVisible();
    });
  });
});
