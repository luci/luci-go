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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, fireEvent } from '@testing-library/react';
import { DocumentSnapshot, onSnapshot } from 'firebase/firestore';

import { useAdminTaskPermission } from '@/fleet/components/actions/shared/use_admin_task_permission';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSSmartRepair } from './chromeos_smart_repair';

jest.mock('@/fleet/components/actions/shared/use_admin_task_permission');
const mockUseAdminTaskPermission = useAdminTaskPermission as jest.Mock;

const mockGetSmartRepair = jest.fn();
jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useFleetConsoleClient: () => ({
    GetSmartRepair: mockGetSmartRepair,
  }),
}));

jest.mock('firebase/firestore', () => {
  const original = jest.requireActual('firebase/firestore');
  return {
    ...original,
    doc: jest.fn().mockReturnValue({}),
    onSnapshot: jest.fn(),
  };
});
const mockOnSnapshot = onSnapshot as jest.Mock;

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: () => ({ id: 'test-device-1' }),
}));

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
}));

describe('<ChromeOSSmartRepair />', () => {
  let queryClient: QueryClient;
  let firestoreCallback: (doc: DocumentSnapshot) => void;

  beforeEach(() => {
    jest.useFakeTimers();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });
    mockGetSmartRepair.mockReset();
    mockUseAdminTaskPermission.mockReset();
    mockOnSnapshot.mockReset();
    mockTrackEvent.mockReset();

    mockOnSnapshot.mockImplementation((_ref, callback) => {
      firestoreCallback = callback;
      return jest.fn(); // unsubscribe
    });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('renders checking permissions by default', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: null });
    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(screen.getByText('Checking permissions...')).toBeInTheDocument();
  });

  it('renders admin required banner if unauthorized', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: false });
    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(screen.getByText('Admin Access Required:')).toBeInTheDocument();
  });

  it('renders idle state when no cached or active run exists', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: true });
    mockGetSmartRepair.mockResolvedValue({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: '',
          alreadyInProgress: false,
          cachedResult: undefined,
        },
      ],
    });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(
      await screen.findByText(
        /No active or cached analysis found for this device/i,
      ),
    ).toBeInTheDocument();
  });

  it('renders active processing run and handles Firestore updates', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: true });
    mockGetSmartRepair.mockResolvedValue({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: 'event-123',
          alreadyInProgress: true,
          cachedResult: undefined,
        },
      ],
    });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    // Initially it should say "Analysis is currently processing."
    expect(
      await screen.findByText(/Analysis is currently processing/i),
    ).toBeInTheDocument();

    // Emit completed Firestore document update
    const mockCompletedDoc = {
      exists: () => true,
      data: () => ({
        status: 'completed',
        result: {
          summary: 'All components functioning normally.',
          conclusions: [
            {
              target: 'DUT',
              status: 'Passed',
              summary: 'Passed all tests',
            },
          ],
          manualRepairActions: ['Check power supply'],
          logsPath: 'gs://test-bucket/logs/2026-05-27-00-00-00/log.txt',
        },
      }),
    };

    await act(async () => {
      firestoreCallback(mockCompletedDoc as unknown as DocumentSnapshot);
    });

    // Verify summary, conclusion and repair actions are visible
    expect(
      screen.getByText('All components functioning normally.'),
    ).toBeInTheDocument();
    expect(screen.getByText('DUT')).toBeInTheDocument();
    expect(screen.getByText('Passed all tests')).toBeInTheDocument();
    expect(screen.getByText('Check power supply')).toBeInTheDocument();
  });

  it('renders and persists error result on failure, and hides idle banner', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: true });

    // Initial check-only call during render
    mockGetSmartRepair.mockResolvedValueOnce({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: 'event-error-123',
          alreadyInProgress: true,
          cachedResult: undefined,
        },
      ],
    });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(
      await screen.findByText(/Analysis is currently processing/i),
    ).toBeInTheDocument();

    // Prepare mock query result for the background refetch that happens on error
    mockGetSmartRepair.mockResolvedValueOnce({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: '',
          alreadyInProgress: false,
          cachedResult: undefined,
        },
      ],
    });

    // Emit error Firestore document update
    const mockErrorDoc = {
      exists: () => true,
      data: () => ({
        status: 'error',
        error: {
          message: 'Smart repair worker failed with code 5',
        },
      }),
    };

    await act(async () => {
      firestoreCallback(mockErrorDoc as unknown as DocumentSnapshot);
    });

    // The customized error message must be visible
    expect(screen.getByText(/Analysis resulted in error/i)).toBeInTheDocument();
    expect(
      screen.getByText(/Smart repair worker failed with code 5/i),
    ).toBeInTheDocument();

    const bugLink = screen.getByRole('link', { name: /file a bug/i });
    expect(bugLink).toBeInTheDocument();
    expect(bugLink.getAttribute('href')).toContain(
      'issuetracker.google.com/issues/new?component=1664178',
    );

    // Idle banner should NOT be shown
    expect(
      screen.queryByText(/No active or cached analysis found for this device/i),
    ).not.toBeInTheDocument();

    // Even after the background refetch resolves and query updates data (meaning eventId becomes empty)
    await act(async () => {
      await queryClient.invalidateQueries({
        queryKey: ['smart-repair', 'test-device-1'],
      });
    });

    // The error banner must STILL be visible
    expect(screen.getByText(/Analysis resulted in error/i)).toBeInTheDocument();
    expect(
      screen.getByText(/Smart repair worker failed with code 5/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: /file a bug/i }),
    ).toBeInTheDocument();

    // Idle banner should STILL be hidden
    expect(
      screen.queryByText(/No active or cached analysis found for this device/i),
    ).not.toBeInTheDocument();
  });

  it('clears the error banner when Retrigger Analysis is clicked', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: true });

    // Initial check-only call
    mockGetSmartRepair.mockResolvedValueOnce({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: 'event-error-123',
          alreadyInProgress: true,
          cachedResult: undefined,
        },
      ],
    });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    // Wait for initial query loading to complete and render the processing state
    expect(
      await screen.findByText(/Analysis is currently processing/i),
    ).toBeInTheDocument();

    // Emit error update
    const mockErrorDoc = {
      exists: () => true,
      data: () => ({
        status: 'error',
        error: {
          message: 'Internal database connection timeout',
        },
      }),
    };

    await act(async () => {
      firestoreCallback(mockErrorDoc as unknown as DocumentSnapshot);
    });

    // Error message is visible
    expect(screen.getByText(/Analysis resulted in error/i)).toBeInTheDocument();
    expect(
      screen.getByText(/Internal database connection timeout/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: /file a bug/i }),
    ).toBeInTheDocument();

    // Mock the retrigger mutation call and subsequent refetch call
    mockGetSmartRepair.mockResolvedValueOnce({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: 'new-event-456',
          alreadyInProgress: false,
          cachedResult: undefined,
        },
      ],
    });

    mockGetSmartRepair.mockResolvedValueOnce({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: 'new-event-456',
          alreadyInProgress: true,
          cachedResult: undefined,
        },
      ],
    });

    // Click Retrigger Analysis
    const retriggerButton = screen.getByRole('button', {
      name: /Retrigger Analysis/i,
    });
    await act(async () => {
      fireEvent.click(retriggerButton);
    });

    // The error banner should immediately be cleared/removed
    expect(
      screen.queryByText(/Analysis resulted in error/i),
    ).not.toBeInTheDocument();
  });

  it('renders the feedback widget when Smart Repair is completed and tracks thumbs up clicks', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: true });
    mockGetSmartRepair.mockResolvedValue({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: 'event-123',
          alreadyInProgress: true,
          cachedResult: undefined,
        },
      ],
    });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(
      await screen.findByText(/Analysis is currently processing/i),
    ).toBeInTheDocument();

    // Emit completed Firestore document update
    const mockCompletedDoc = {
      exists: () => true,
      data: () => ({
        status: 'completed',
        result: {
          summary: 'All components functioning normally.',
          conclusions: [],
          manualRepairActions: [],
          logsPath: 'gs://test-bucket/logs/2026-05-27-00-00-00/log.txt',
        },
      }),
    };

    await act(async () => {
      firestoreCallback(mockCompletedDoc as unknown as DocumentSnapshot);
    });

    // Verify feedback widget is visible
    expect(screen.getByText('Did this save time?')).toBeInTheDocument();

    const thumbsUpButton = screen.getByRole('button', {
      name: 'Yes, it saved time',
    });
    expect(thumbsUpButton).toBeInTheDocument();

    const thumbsDownButton = screen.getByRole('button', {
      name: 'No, it did not',
    });
    expect(thumbsDownButton).toBeInTheDocument();

    // Click thumbs up
    await act(async () => {
      fireEvent.click(thumbsUpButton);
    });

    // Verify feedback submission calls GA trackEvent and updates UI
    expect(mockTrackEvent).toHaveBeenCalledTimes(2);
    expect(mockTrackEvent).toHaveBeenNthCalledWith(
      1,
      'smart_repair_tab_viewed',
      {
        componentName: 'smart_repair_tab',
      },
    );
    expect(mockTrackEvent).toHaveBeenNthCalledWith(2, 'smart_repair_feedback', {
      eventId: 'event-123',
      feedback: 'up',
    });

    expect(screen.getByText('Thank you!')).toBeInTheDocument();
  });

  it('renders the feedback widget and tracks thumbs down clicks', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: true });
    mockGetSmartRepair.mockResolvedValue({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: 'event-123',
          alreadyInProgress: true,
          cachedResult: undefined,
        },
      ],
    });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(
      await screen.findByText(/Analysis is currently processing/i),
    ).toBeInTheDocument();

    const mockCompletedDoc = {
      exists: () => true,
      data: () => ({
        status: 'completed',
        result: {
          summary: 'All components functioning normally.',
          conclusions: [],
          manualRepairActions: [],
          logsPath: 'gs://test-bucket/logs/2026-05-27-00-00-00/log.txt',
        },
      }),
    };

    await act(async () => {
      firestoreCallback(mockCompletedDoc as unknown as DocumentSnapshot);
    });

    const thumbsDownButton = screen.getByRole('button', {
      name: 'No, it did not',
    });

    // Click thumbs down
    await act(async () => {
      fireEvent.click(thumbsDownButton);
    });

    expect(mockTrackEvent).toHaveBeenCalledTimes(2);
    expect(mockTrackEvent).toHaveBeenNthCalledWith(
      1,
      'smart_repair_tab_viewed',
      {
        componentName: 'smart_repair_tab',
      },
    );
    expect(mockTrackEvent).toHaveBeenNthCalledWith(2, 'smart_repair_feedback', {
      eventId: 'event-123',
      feedback: 'down',
    });

    expect(screen.getByText('Thank you!')).toBeInTheDocument();
  });

  it('tracks smart_repair_tab_viewed when rendered with admin permissions', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: true });
    mockGetSmartRepair.mockResolvedValue({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: '',
          alreadyInProgress: false,
          cachedResult: undefined,
        },
      ],
    });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    // Should call trackEvent with 'smart_repair_tab_viewed'
    expect(mockTrackEvent).toHaveBeenCalledTimes(1);
    expect(mockTrackEvent).toHaveBeenCalledWith('smart_repair_tab_viewed', {
      componentName: 'smart_repair_tab',
    });
  });

  it('does not track smart_repair_tab_viewed when rendered without admin permissions', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: false });
    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    // Should NOT call trackEvent
    expect(mockTrackEvent).not.toHaveBeenCalled();
  });

  it('tracks run_smart_repair when Retrigger Analysis is clicked', async () => {
    mockUseAdminTaskPermission.mockReturnValue({ hasPermission: true });
    mockGetSmartRepair.mockResolvedValue({
      results: [
        {
          deviceId: 'test-device-1',
          eventId: '',
          alreadyInProgress: false,
          cachedResult: undefined,
        },
      ],
    });

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <ChromeOSSmartRepair />
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    // Wait for query to resolve so button is not disabled
    expect(
      await screen.findByText(/No active or cached analysis found/i),
    ).toBeInTheDocument();

    const retriggerButton = screen.getByRole('button', {
      name: /Retrigger Analysis/i,
    });

    // Reset trackEvent calls mock to ignore the mount event
    mockTrackEvent.mockClear();

    await act(async () => {
      fireEvent.click(retriggerButton);
    });

    expect(mockTrackEvent).toHaveBeenCalledTimes(1);
    expect(mockTrackEvent).toHaveBeenCalledWith('run_smart_repair', {
      componentName: 'retrigger_analysis_button',
    });
  });
});
