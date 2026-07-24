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

import { beforeEach, describe, expect, it, jest } from '@jest/globals';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { ReactNode } from 'react';

import { ReadWorkPlanRequest } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_request.pb';
import { ReadWorkPlanResponse } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_response.pb';
import { StageAttemptState } from '@/proto/turboci/graph/orchestrator/v1/stage_attempt_state.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { fetchGrpcWeb } from '../grpc_query';

import { usePaginatedReadWorkPlan } from './turbo_ci';

// Mock fetchGrpcWeb
jest.mock('../grpc_query', () => {
  const originalModule = jest.requireActual('../grpc_query') as object;
  return {
    ...originalModule,
    fetchGrpcWeb: jest.fn(),
  };
});

const mockFetchGrpcWeb = fetchGrpcWeb as jest.MockedFunction<
  typeof fetchGrpcWeb
>;

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
    },
  },
});

const wrapper = ({ children }: { children: ReactNode }) => (
  <FakeContextProvider>
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  </FakeContextProvider>
);

describe('usePaginatedReadWorkPlan', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    queryClient.clear();
  });

  it('aggregates multiple pages of workplan data', async () => {
    // Page 1 mock response
    const page1Response = ReadWorkPlanResponse.fromPartial({
      workplan: {
        identifier: { id: 'wp-1' },
        stages: [{ identifier: { id: 'stage-1' } }],
        checks: [{ identifier: { id: 'check-1' } }],
      },
      valueData: {
        'digest-1': ValueData.fromPartial({ json: { value: '{"data": 1}' } }),
      },
      paginationToken: 'token-page-2',
      currentAttemptState: {
        state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
      },
      version: { ts: '2026-07-21T00:00:00Z' },
    });

    // Page 2 mock response (final page)
    const page2Response = ReadWorkPlanResponse.fromPartial({
      workplan: {
        identifier: { id: 'wp-1' },
        stages: [{ identifier: { id: 'stage-2' } }],
        checks: [{ identifier: { id: 'check-2' } }],
      },
      valueData: {
        'digest-2': ValueData.fromPartial({ json: { value: '{"data": 2}' } }),
      },
      paginationToken: '',
      currentAttemptState: {
        state: StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE,
      },
      version: { ts: '2026-07-21T01:00:00Z' },
    });

    mockFetchGrpcWeb
      .mockResolvedValueOnce(page1Response)
      .mockResolvedValueOnce(page2Response);

    const request = ReadWorkPlanRequest.fromPartial({
      workplanId: { id: 'wp-1' },
    });

    const { result } = renderHook(
      () => usePaginatedReadWorkPlan(request, 'https://test-host'),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockFetchGrpcWeb).toHaveBeenCalledTimes(2);

    // Verify first call request
    expect(mockFetchGrpcWeb).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({
        request: expect.objectContaining({
          paginationToken: undefined,
        }),
      }),
    );

    // Verify second call request uses token from page 1
    expect(mockFetchGrpcWeb).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        request: expect.objectContaining({
          paginationToken: 'token-page-2',
        }),
      }),
    );

    // Verify aggregated response data
    const data = result.current.data;
    expect(data).toBeDefined();
    expect(data?.workplan?.stages).toHaveLength(2);
    expect(data?.workplan?.stages?.[0]?.identifier?.id).toBe('stage-1');
    expect(data?.workplan?.stages?.[1]?.identifier?.id).toBe('stage-2');

    expect(data?.workplan?.checks).toHaveLength(2);
    expect(data?.workplan?.checks?.[0]?.identifier?.id).toBe('check-1');
    expect(data?.workplan?.checks?.[1]?.identifier?.id).toBe('check-2');

    expect(data?.valueData).toEqual({
      'digest-1': expect.objectContaining({ json: { value: '{"data": 1}' } }),
      'digest-2': expect.objectContaining({ json: { value: '{"data": 2}' } }),
    });

    expect(data?.version?.ts).toBe('2026-07-21T01:00:00Z');
  });
});
