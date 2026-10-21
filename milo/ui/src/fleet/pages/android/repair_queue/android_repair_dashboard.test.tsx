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
import { fireEvent, render, screen } from '@testing-library/react';

import * as FeatureFlagsModule from '@/common/feature_flags';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import * as PrpcClientsModule from '@/fleet/hooks/prpc_clients';
import * as UseAndroidRepairQueueModule from '@/fleet/pages/android/repair_queue/use_android_repair_queue';
import { getAndroidRepairQueueRowId } from '@/fleet/pages/android/repair_queue/use_android_repair_queue_columns';
import * as UseClaimAndroidRepairTaskModule from '@/fleet/pages/android/repair_queue/use_claim_android_repair_task';
import { Component as AndroidRepairsPageComponent } from '@/fleet/pages/android/repairs/repairs_page';
import { createMockUseFleetConsoleClient } from '@/fleet/testing_tools/mocks/fleet_console_client';
import {
  getMobileHarnessDeviceListLink,
  getMobileHarnessHostListLink,
} from '@/fleet/utils/mobile_harness';
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import {
  AndroidMachineType,
  AndroidRepairQueueItem,
  AndroidWorkspace,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidRepairDashboard } from './android_repair_dashboard';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  ...jest.requireActual('@/generic_libs/components/google_analytics'),
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
  TrackLeafRoutePageView: ({ children }: { children: React.ReactNode }) =>
    children,
}));

const MOCK_QUEUE_ITEMS: readonly AndroidRepairQueueItem[] = [
  {
    deviceId: 'android-device-101',
    fcMachineType: AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE,
    hostGroups: ['group1', 'group2'],
    runTarget: 'seahawk',
    state: 'MISSING',
    taskId: '101',
    claimedBy: '',
    claimedAt: undefined,
    hostGroupHealthPct: 0.824,
    runTargetHealthPct: 0.95,
    priorityScore: '250',
    labName: 'MTV-1',
    matchedRuleIds: ['1'],
    etag: 'etag-101',
  },
  {
    deviceId: 'android-device-102',
    fcMachineType: AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE,
    hostGroups: ['group3'],
    runTarget: 'mustang',
    state: 'FAILED',
    taskId: '102',
    claimedBy: 'user@example.com',
    claimedAt: '2026-09-16T10:00:00Z',
    hostGroupHealthPct: undefined,
    runTargetHealthPct: undefined,
    priorityScore: '50',
    labName: 'MTV-2',
    matchedRuleIds: [],
    etag: 'etag-102',
  },
];

// A device and a host sharing the same `deviceId` and no `taskId`; the
// table must still key them as distinct rows via `fcMachineType`.
const MOCK_SAME_ID_ITEMS: readonly AndroidRepairQueueItem[] = [
  {
    ...MOCK_QUEUE_ITEMS[0],
    deviceId: 'shared-id-1',
    fcMachineType: AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE,
    taskId: '0',
    labName: 'LAB-DEVICE',
  },
  {
    ...MOCK_QUEUE_ITEMS[0],
    deviceId: 'shared-id-1',
    fcMachineType: AndroidMachineType.ANDROID_MACHINE_TYPE_HOST,
    taskId: '0',
    labName: 'LAB-HOST',
  },
];

describe('<AndroidRepairDashboard />', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });

    jest
      .spyOn(PrpcClientsModule, 'useFleetConsoleClient')
      .mockImplementation(createMockUseFleetConsoleClient());
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  const renderDashboard = (workspace: AndroidPageWorkspace = 'Android') =>
    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider
          mountedPath="/p/:platform/repairs"
          routerOptions={{
            initialEntries: [
              workspace === 'Pixel' ? '/p/pixel/repairs' : '/p/android/repairs',
            ],
          }}
        >
          <SettingsProvider>
            <ShortcutProvider>
              <AndroidRepairDashboard workspace={workspace} />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

  it('renders all columns for Android and hides Host Group for Pixel', () => {
    jest
      .spyOn(UseAndroidRepairQueueModule, 'useAndroidRepairQueue')
      .mockReturnValue({
        data: {
          repairQueueItems: MOCK_QUEUE_ITEMS,
          totalSize: 2,
          nextPageToken: '',
        },
        inProgressCount: 1,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseAndroidRepairQueueModule.useAndroidRepairQueue
      >);

    const { unmount } = renderDashboard('Android');

    expect(
      screen.getByText('Android Manual Repair Dashboard'),
    ).toBeInTheDocument();
    expect(screen.getByText(/My tasks \(\d+\):/)).toBeInTheDocument();
    expect(screen.getByText('Show devices')).toBeInTheDocument();
    expect(screen.getByText('Rank')).toBeInTheDocument();
    expect(screen.getByText('Id')).toBeInTheDocument();
    expect(screen.getByText('Machine Type')).toBeInTheDocument();
    expect(screen.getByText('Lab')).toBeInTheDocument();
    expect(screen.getByText('Host Group')).toBeInTheDocument();
    expect(screen.getByText('Run Target')).toBeInTheDocument();
    expect(
      screen.getByText('Host Group / Run Target Health'),
    ).toBeInTheDocument();
    expect(screen.getByText('State')).toBeInTheDocument();
    expect(screen.getByText('Priority Score')).toBeInTheDocument();
    expect(screen.getByText('Assignee')).toBeInTheDocument();
    expect(screen.getByText('MTV-1')).toBeInTheDocument();
    expect(screen.getByText('MTV-2')).toBeInTheDocument();
    expect(screen.getByText('+250 pts')).toBeInTheDocument();
    expect(screen.getByText('+50 pts')).toBeInTheDocument();

    unmount();

    renderDashboard('Pixel');

    expect(
      screen.getByText('Pixel Manual Repair Dashboard'),
    ).toBeInTheDocument();
    expect(screen.getByText('Rank')).toBeInTheDocument();
    expect(screen.getByText('Id')).toBeInTheDocument();
    expect(screen.getByText('Machine Type')).toBeInTheDocument();
    expect(screen.getByText('Lab')).toBeInTheDocument();
    expect(screen.queryByText('Host Group')).not.toBeInTheDocument();
    expect(screen.getByText('Run Target')).toBeInTheDocument();
    expect(
      screen.getByText('Host Group / Run Target Health'),
    ).toBeInTheDocument();
    expect(screen.getByText('State')).toBeInTheDocument();
    expect(screen.getByText('Priority Score')).toBeInTheDocument();
    expect(screen.getByText('Assignee')).toBeInTheDocument();
  });

  it('triggers claim and unclaim mutations when clicking Assignee buttons/avatars', () => {
    const mockClaim = jest.fn();
    const mockUnclaim = jest.fn();

    jest
      .spyOn(UseClaimAndroidRepairTaskModule, 'useClaimAndroidRepairTask')
      .mockReturnValue({
        mutate: mockClaim,
        isPending: false,
      } as unknown as ReturnType<
        typeof UseClaimAndroidRepairTaskModule.useClaimAndroidRepairTask
      >);

    jest
      .spyOn(UseClaimAndroidRepairTaskModule, 'useUnclaimAndroidRepairTask')
      .mockReturnValue({
        mutate: mockUnclaim,
        isPending: false,
      } as unknown as ReturnType<
        typeof UseClaimAndroidRepairTaskModule.useUnclaimAndroidRepairTask
      >);

    jest
      .spyOn(UseAndroidRepairQueueModule, 'useAndroidRepairQueue')
      .mockReturnValue({
        data: {
          repairQueueItems: MOCK_QUEUE_ITEMS,
          totalSize: 2,
          nextPageToken: '',
        },
        inProgressCount: 1,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseAndroidRepairQueueModule.useAndroidRepairQueue
      >);

    renderDashboard('Android');

    const claimButton = screen.getByRole('button', { name: 'Claim' });
    fireEvent.click(claimButton);
    expect(mockClaim).toHaveBeenCalledWith({ taskId: '101', etag: 'etag-101' });

    const avatar = screen.getByText('U');
    fireEvent.click(avatar);
    expect(mockUnclaim).toHaveBeenCalledWith({
      taskId: '102',
      etag: 'etag-102',
    });
  });

  it('keys device and host rows sharing a deviceId separately and links hosts to the MH lab list', () => {
    jest
      .spyOn(UseAndroidRepairQueueModule, 'useAndroidRepairQueue')
      .mockReturnValue({
        data: {
          repairQueueItems: MOCK_SAME_ID_ITEMS,
          totalSize: 2,
          nextPageToken: '',
        },
        inProgressCount: 0,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseAndroidRepairQueueModule.useAndroidRepairQueue
      >);

    renderDashboard('Android');

    // Both rows render: no duplicate-key collapse on the shared deviceId.
    expect(MOCK_SAME_ID_ITEMS.map(getAndroidRepairQueueRowId)).toEqual([
      'shared-id-1/device',
      'shared-id-1/host',
    ]);
    expect(screen.getByText('LAB-DEVICE')).toBeInTheDocument();
    expect(screen.getByText('LAB-HOST')).toBeInTheDocument();
    expect(screen.getByText('device')).toBeInTheDocument();
    expect(screen.getByText('host')).toBeInTheDocument();

    const mhLinks = screen
      .getAllByRole('link')
      .map((link) => link.getAttribute('href') ?? '')
      .filter((href) => href.startsWith('https://mobileharness-fe'));
    expect(mhLinks).toEqual([
      getMobileHarnessDeviceListLink('shared-id-1'),
      getMobileHarnessHostListLink('shared-id-1'),
    ]);
    expect(mhLinks[1]).toContain('/lablistview?');
  });

  it('scopes the queue request to the selected workspace', () => {
    const mockUseQueue = jest
      .spyOn(UseAndroidRepairQueueModule, 'useAndroidRepairQueue')
      .mockReturnValue({
        data: {
          repairQueueItems: [],
          totalSize: 0,
          nextPageToken: '',
        },
        inProgressCount: 0,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseAndroidRepairQueueModule.useAndroidRepairQueue
      >);

    const { unmount } = renderDashboard('Pixel');

    expect(mockUseQueue).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace: AndroidWorkspace.ANDROID_WORKSPACE_PIXEL,
        filter: '',
      }),
    );
    // The "My tasks" actions fetch the current user's claimed tasks in the
    // same workspace.
    const allRequests = mockUseQueue.mock.calls.map(([request]) => request);
    const myTasksRequests = allRequests.filter((request) =>
      request.filter.startsWith('claimed_by = '),
    );
    expect(myTasksRequests.length).toBeGreaterThan(0);
    for (const request of myTasksRequests) {
      expect(request.workspace).toBe(AndroidWorkspace.ANDROID_WORKSPACE_PIXEL);
    }
    // The queue table issues its own (paged) request for the same workspace.
    const tableRequests = allRequests.filter(
      (request) => !myTasksRequests.includes(request),
    );
    expect(tableRequests.length).toBeGreaterThan(0);
    for (const request of tableRequests) {
      expect(request.workspace).toBe(AndroidWorkspace.ANDROID_WORKSPACE_PIXEL);
      expect(request.filter).toBe('');
    }
    expect(mockUseQueue).not.toHaveBeenCalledWith(
      expect.objectContaining({
        workspace: AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
      }),
    );

    unmount();
    mockUseQueue.mockClear();

    renderDashboard('Android');

    expect(mockUseQueue).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace: AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
        filter: '',
      }),
    );
  });

  describe('RepairsPage feature flag routing', () => {
    const renderRepairsPage = (initialUrl = '/p/android/repairs') =>
      render(
        <QueryClientProvider client={queryClient}>
          <FakeContextProvider
            mountedPath="/p/:platform/repairs"
            routerOptions={{
              initialEntries: [initialUrl],
            }}
          >
            <SettingsProvider>
              <ShortcutProvider>
                <AndroidRepairsPageComponent workspace="Android" />
              </ShortcutProvider>
            </SettingsProvider>
          </FakeContextProvider>
        </QueryClientProvider>,
      );

    it('renders RepairListPage when enableAndroidRepairQueue is false', () => {
      jest.spyOn(FeatureFlagsModule, 'useFeatureFlag').mockReturnValue(false);

      renderRepairsPage();

      expect(screen.getByText('Repair metrics')).toBeInTheDocument();
    });

    it('renders AndroidRepairDashboard when enableAndroidRepairQueue is true', () => {
      jest.spyOn(FeatureFlagsModule, 'useFeatureFlag').mockReturnValue(true);

      jest
        .spyOn(UseAndroidRepairQueueModule, 'useAndroidRepairQueue')
        .mockReturnValue({
          data: {
            repairQueueItems: MOCK_QUEUE_ITEMS,
            totalSize: 2,
            nextPageToken: '',
          },
          inProgressCount: 1,
          isPending: false,
          isError: false,
          isFetching: false,
          isLoading: false,
          isPlaceholderData: false,
        } as unknown as ReturnType<
          typeof UseAndroidRepairQueueModule.useAndroidRepairQueue
        >);

      renderRepairsPage();

      expect(
        screen.getByText('Android Manual Repair Dashboard'),
      ).toBeInTheDocument();
    });
  });
});
