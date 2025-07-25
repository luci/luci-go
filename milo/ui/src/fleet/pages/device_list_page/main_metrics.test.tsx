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

import { UseQueryResult } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';

import { CountDevicesResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { MainMetrics, MainMetricsContainer } from './main_metrics';

const MOCK_DATA: CountDevicesResponse = {
  total: 440,
  taskState: {
    busy: 401,
    idle: 40,
  },
  deviceState: {
    ready: 40,
    needManualRepair: 40,
    needRepair: 40,
    repairFailed: 40,
    needsDeploy: 0,
    needsReplacement: 0,
  },
};

describe('<MainMetrics />', () => {
  it('should render', async () => {
    const mockQuery = (
      data: CountDevicesResponse,
    ): UseQueryResult<CountDevicesResponse, unknown> => ({
      data,
      isLoading: false,
      error: null,
      isError: false,
      isSuccess: true,
      isPending: false,
      status: 'success',
      fetchStatus: 'idle',
      refetch: jest.fn(),
      isPlaceholderData: false,
      isFetched: true,
      isFetchedAfterMount: true,
      isFetching: false,
      isInitialLoading: false,
      isLoadingError: false,
      isRefetchError: false,
      isRefetching: false,
      isStale: false,
      dataUpdatedAt: 0,
      errorUpdatedAt: 0,
      failureCount: 0,
      failureReason: null,
      errorUpdateCount: 0,
      isPaused: false,
      promise: Promise.resolve(data),
    });

    render(
      <FakeContextProvider>
        <MainMetrics
          countQuery={mockQuery(MOCK_DATA)}
          labstationsQuery={mockQuery(MOCK_DATA)}
          needsDeployLabstationsQuery={mockQuery(MOCK_DATA)}
        />
      </FakeContextProvider>,
    );

    const total = screen.getByText('Total');
    expect(total).toBeVisible();
  });
});

describe('<MainMetricsContainer />', () => {
  it('should render', async () => {
    render(
      <FakeContextProvider>
        <MainMetricsContainer selectedOptions={{}} />
      </FakeContextProvider>,
    );

    const total = screen.getByText('Total');
    expect(total).toBeVisible();
  });
});
