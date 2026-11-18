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
import { render, screen, waitFor } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import * as PrpcClientsModule from '@/fleet/hooks/prpc_clients';
import { createMockUseFleetConsoleClient } from '@/fleet/testing_tools/mocks/fleet_console_client';
import {
  AndroidMachineType,
  AndroidWorkspace,
  ListAndroidRepairQueueRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidRepairQueueTable } from './android_repair_queue_table';

describe('<AndroidRepairQueueTable />', () => {
  let queryClient: QueryClient;
  let mockListQueue: jest.Mock;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    mockListQueue = jest.fn().mockResolvedValue({
      repairQueueItems: [
        {
          deviceId: 'android-dut-1',
          fcMachineType: AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE,
          hostGroups: ['group-a'],
          runTarget: 'mock-target',
          state: 'DYING',
          taskId: '101',
          claimedBy: '',
          claimedAt: undefined,
          hostGroupHealthPct: 0.8,
          runTargetHealthPct: 0.9,
          priorityScore: '200',
          labName: 'mock-lab',
          matchedRuleIds: [],
          etag: 'etag-1',
          rank: 1,
        },
      ],
      totalSize: 1,
      inProgressCount: 0,
      nextPageToken: '',
    });

    const baseClient = createMockUseFleetConsoleClient()();
    jest.spyOn(PrpcClientsModule, 'useFleetConsoleClient').mockReturnValue({
      ...baseClient,
      ListAndroidRepairQueue: {
        query: (req: ListAndroidRepairQueueRequest) => ({
          queryKey: ['ListAndroidRepairQueue', req],
          queryFn: () => mockListQueue(req),
        }),
      },
    } as unknown as ReturnType<typeof PrpcClientsModule.useFleetConsoleClient>);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  const renderTable = (
    props: Partial<React.ComponentProps<typeof AndroidRepairQueueTable>>,
  ) =>
    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <SettingsProvider>
            <ShortcutProvider>
              <AndroidRepairQueueTable workspace="Android" {...props} />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

  it('disables ListAndroidRepairQueue while siteFilter is null', () => {
    renderTable({
      siteId: 'SITE-ALPHA',
      siteFilter: null,
      filter: 'state = "DYING"',
    });

    expect(mockListQueue).not.toHaveBeenCalled();
  });

  it('combines siteFilter and filter and forwards siteId to ListAndroidRepairQueue', async () => {
    renderTable({
      siteId: 'SITE-ALPHA',
      siteFilter: 'lab_name = "mock-lab"',
      filter: 'state = "DYING"',
    });

    expect(await screen.findByText('android-dut-1')).toBeInTheDocument();
    await waitFor(() =>
      expect(mockListQueue).toHaveBeenCalledWith(
        expect.objectContaining({
          workspace: AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
          siteId: 'SITE-ALPHA',
          filter: '(lab_name = "mock-lab") AND (state = "DYING")',
        }),
      ),
    );
  });

  it('forwards filter without a site wrapper when siteFilter is empty', async () => {
    renderTable({
      siteId: '',
      siteFilter: '',
      filter: 'state = "DYING"',
    });

    expect(await screen.findByText('android-dut-1')).toBeInTheDocument();
    await waitFor(() =>
      expect(mockListQueue).toHaveBeenCalledWith(
        expect.objectContaining({
          workspace: AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
          siteId: '',
          filter: 'state = "DYING"',
        }),
      ),
    );
  });
});
