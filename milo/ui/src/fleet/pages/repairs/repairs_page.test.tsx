// Copyright 2025 The LUCI Authors.
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
import { act, render, screen } from '@testing-library/react';

import * as PrpcClients from '@/fleet/hooks/prpc_clients';
import { createMockUseFleetConsoleClient } from '@/fleet/testing_tools/mocks/fleet_console_client';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { RepairListPage } from './repairs_page';

describe('RepairListPage', () => {
  let queryClient: QueryClient;
  let mockUseFleetConsoleClient: jest.SpyInstance;

  beforeEach(() => {
    jest.useFakeTimers();
    queryClient = new QueryClient();

    mockUseFleetConsoleClient = jest
      .spyOn(PrpcClients, 'useFleetConsoleClient')
      .mockImplementation(createMockUseFleetConsoleClient());

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <RepairListPage platform={Platform.ANDROID} />
        </FakeContextProvider>
      </QueryClientProvider>,
    );
    act(() => {
      jest.runAllTimers();
    });
  });

  it('renders the component and calls the mock', () => {
    expect(screen.getByText('Repair metrics')).toBeInTheDocument();
    expect(screen.getByText('Offline / Total Devices')).toBeInTheDocument();
    expect(mockUseFleetConsoleClient).toHaveBeenCalled();
  });
});
