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

import { renderHook, waitFor } from '@testing-library/react';

import * as gapiQueryHooks from '@/common/hooks/gapi_query/gapi_query';
import { API_V1_BASE_PATH as BASE_PATH } from '@/crystal_ball/constants';
import {
  TypedDashboardStateOperation,
  useCreateDashboardState,
  useGetDashboardState,
  useListDashboardStates,
  useListDashboardStatesInfinite,
  useUpdateDashboardState,
  useDeleteDashboardState,
  useUndeleteDashboardState,
  useListDashboardStateRevisions,
  useGetDashboardStateRevision,
  useRollbackDashboardState,
  useGenerateDashboardState,
} from '@/crystal_ball/hooks';
import {
  createMockInfiniteQueryResult,
  createMockMutationResult,
  createMockQueryResult,
} from '@/crystal_ball/tests';
import {
  DashboardState,
  CreateDashboardStateRequest,
  UpdateDashboardStateRequest,
  GetDashboardStateRequest,
  ListDashboardStatesRequest,
  DeleteDashboardStateRequest,
  UndeleteDashboardStateRequest,
  ListDashboardStateRevisionsRequest,
  GetDashboardStateRevisionRequest,
  RollbackDashboardStateRequest,
  GenerateDashboardStateRequest,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

// Mock the imported hooks
jest.mock('@/common/hooks/gapi_query/gapi_query', () => ({
  useGapiQuery: jest.fn(),
  useInfiniteGapiQuery: jest.fn(),
  useGapiMutation: jest.fn(),
}));

const mockedUseGapiQuery = jest.mocked(gapiQueryHooks.useGapiQuery);
const mockedUseInfiniteGapiQuery = jest.mocked(
  gapiQueryHooks.useInfiniteGapiQuery,
);
const mockedUseGapiMutation = jest.mocked(gapiQueryHooks.useGapiMutation);

describe('use_dashboard_state_api', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  // Example minimal DashboardState for testing
  const sampleDashboardState: DashboardState = DashboardState.fromPartial({
    name: 'dashboardStates/test-id',
    displayName: 'Test Dashboard',
    dashboardContent: {
      widgets: [],
      dataSpecs: {},
      globalFilters: [],
    },
  });

  const sampleOperation: TypedDashboardStateOperation = {
    name: 'operations/test-operation-id',
    done: true,
    response: sampleDashboardState,
  };

  describe('useCreateDashboardState', () => {
    const request = CreateDashboardStateRequest.fromPartial({
      dashboardState: {
        dashboardContent: {
          widgets: [],
          dataSpecs: {},
          globalFilters: [],
        },
        displayName: 'New Dash',
      },
      dashboardStateId: 'new-dash-id',
    });

    it('should call useGapiMutation with correct arguments', () => {
      // Mock mutateAsync to resolve with our strongly typed operation
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        }),
      );
      renderHook(() => useCreateDashboardState(), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiMutation).toHaveBeenCalledTimes(1);

      // Extract the request builder function passed to useGapiMutation
      const requestBuilder = mockedUseGapiMutation.mock.calls[0][0];
      const builtRequest = requestBuilder(request);

      expect(builtRequest).toEqual({
        path: `${BASE_PATH}/dashboardStates`,
        method: 'POST',
        body: request.dashboardState,
        params: {
          dashboardStateId: request.dashboardStateId,
          validateOnly: false,
          requestId: '',
        },
      });
    });

    it('should pass through options', () => {
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        }),
      );
      const options = { mutationKey: ['test'] };
      renderHook(() => useCreateDashboardState(options), {
        wrapper: FakeContextProvider,
      });
      expect(mockedUseGapiMutation).toHaveBeenCalledWith(
        expect.any(Function),
        options,
      );
    });
  });

  describe('useGetDashboardState', () => {
    const request = GetDashboardStateRequest.fromPartial({
      name: 'dashboardStates/test-id',
    });

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue(
        createMockQueryResult(sampleDashboardState),
      );
      renderHook(() => useGetDashboardState(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.name}`,
          method: 'GET',
        },
        undefined,
      );
    });

    it('should return mocked DashboardState', async () => {
      mockedUseGapiQuery.mockReturnValue(
        createMockQueryResult(sampleDashboardState),
      );
      const { result } = renderHook(() => useGetDashboardState(request), {
        wrapper: FakeContextProvider,
      });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual(sampleDashboardState);
    });
  });

  describe('useListDashboardStates', () => {
    const request = ListDashboardStatesRequest.fromPartial({ pageSize: 10 });

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue(
        createMockQueryResult({ dashboardStates: [sampleDashboardState] }),
      );
      renderHook(() => useListDashboardStates(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/dashboardStates`,
          method: 'GET',
          params: request,
        },
        undefined,
      );
    });
  });

  describe('useListDashboardStatesInfinite', () => {
    const request = { pageSize: 20, filter: '', showDeleted: false };

    it('should call useInfiniteGapiQuery with correct arguments', () => {
      mockedUseInfiniteGapiQuery.mockReturnValue(
        createMockInfiniteQueryResult({
          pages: [
            {
              dashboardStates: [sampleDashboardState],
              nextPageToken: '',
            },
          ],
          pageParams: [],
        }),
      );

      renderHook(() => useListDashboardStatesInfinite(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseInfiniteGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseInfiniteGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/dashboardStates`,
          method: 'GET',
          params: request,
        },
        undefined,
      );
    });

    it('should pass through options', () => {
      mockedUseInfiniteGapiQuery.mockReturnValue(
        createMockInfiniteQueryResult({ pages: [], pageParams: [] }),
      );
      const options = { gcTime: 50000 };

      renderHook(() => useListDashboardStatesInfinite(request, options), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseInfiniteGapiQuery).toHaveBeenCalledWith(
        expect.any(Object),
        options,
      );
    });
  });

  describe('useUpdateDashboardState', () => {
    const request = UpdateDashboardStateRequest.fromPartial({
      dashboardState: sampleDashboardState,
      updateMask: ['displayName'],
    });

    it('should call useGapiMutation with correct arguments', () => {
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        }),
      );
      renderHook(() => useUpdateDashboardState(), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiMutation).toHaveBeenCalledTimes(1);

      const requestBuilder = mockedUseGapiMutation.mock.calls[0][0];
      const builtRequest = requestBuilder(request);

      expect(builtRequest).toEqual({
        path: `${BASE_PATH}/${sampleDashboardState.name}`,
        method: 'PATCH',
        body: request.dashboardState,
        params: {
          updateMask: 'displayName',
          allowMissing: undefined,
          validateOnly: false,
          requestId: '',
        },
      });
    });

    it('should pass through options', () => {
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        }),
      );
      const options = { mutationKey: ['test-update'] };
      renderHook(() => useUpdateDashboardState(options), {
        wrapper: FakeContextProvider,
      });
      expect(mockedUseGapiMutation).toHaveBeenCalledWith(
        expect.any(Function),
        options,
      );
    });
  });

  describe('useDeleteDashboardState', () => {
    const request = DeleteDashboardStateRequest.fromPartial({
      name: 'dashboardStates/test-id',
    });

    it('should call useGapiMutation with correct arguments', () => {
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        }),
      );
      renderHook(() => useDeleteDashboardState(), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiMutation).toHaveBeenCalledTimes(1);

      const requestBuilder = mockedUseGapiMutation.mock.calls[0][0];
      const builtRequest = requestBuilder(request);

      expect(builtRequest).toEqual({
        path: `${BASE_PATH}/${request.name}`,
        method: 'DELETE',
        params: {
          etag: '',
          validateOnly: false,
          allowMissing: false,
          requestId: '',
        },
      });
    });

    it('should pass through options', () => {
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        }),
      );
      const options = { mutationKey: ['test-delete'] };
      renderHook(() => useDeleteDashboardState(options), {
        wrapper: FakeContextProvider,
      });
      expect(mockedUseGapiMutation).toHaveBeenCalledWith(
        expect.any(Function),
        options,
      );
    });
  });

  describe('useUndeleteDashboardState', () => {
    const request = UndeleteDashboardStateRequest.fromPartial({
      name: 'dashboardStates/test-id',
      etag: 'test-etag',
      validateOnly: true,
      requestId: 'req-123',
    });

    it('should call useGapiMutation with correct arguments', () => {
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        }),
      );
      renderHook(() => useUndeleteDashboardState(), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiMutation).toHaveBeenCalledTimes(1);

      const requestBuilder = mockedUseGapiMutation.mock.calls[0][0];
      const builtRequest = requestBuilder(request);

      expect(builtRequest).toEqual({
        path: `${BASE_PATH}/${request.name}:undelete`,
        method: 'POST',
        body: {
          etag: request.etag,
          validateOnly: request.validateOnly,
          requestId: request.requestId,
        },
        params: {},
      });
    });

    it('should pass through options', () => {
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        }),
      );
      const options = { mutationKey: ['test-undelete'] };
      renderHook(() => useUndeleteDashboardState(options), {
        wrapper: FakeContextProvider,
      });
      expect(mockedUseGapiMutation).toHaveBeenCalledWith(
        expect.any(Function),
        options,
      );
    });
  });

  describe('useListDashboardStateRevisions', () => {
    const request = ListDashboardStateRevisionsRequest.fromPartial({
      name: 'dashboardStates/test-id',
    });

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue(
        createMockQueryResult({ dashboardStates: [sampleDashboardState] }),
      );
      renderHook(() => useListDashboardStateRevisions(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.name}/revisions`,
          method: 'GET',
          params: {
            pageSize: 0,
            pageToken: '',
          },
        },
        undefined,
      );
    });
  });

  describe('useGetDashboardStateRevision', () => {
    const request = GetDashboardStateRevisionRequest.fromPartial({
      name: 'dashboardStates/test-id/revisions/rev1',
    });

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue(
        createMockQueryResult(sampleDashboardState),
      );
      renderHook(() => useGetDashboardStateRevision(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.name}`,
          method: 'GET',
        },
        undefined,
      );
    });
  });

  describe('useRollbackDashboardState', () => {
    const request = RollbackDashboardStateRequest.fromPartial({
      name: 'dashboardStates/test-id',
      revisionId: 'rev1',
    });

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue(
        createMockQueryResult(sampleDashboardState),
      );
      renderHook(() => useRollbackDashboardState(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.name}:rollback`,
          method: 'POST',
          body: request,
        },
        undefined,
      );
    });
  });

  describe('useGenerateDashboardState', () => {
    const request = GenerateDashboardStateRequest.fromPartial({
      prompt: 'A test prompt',
      metricKeys: ['metric1', 'metric2'],
    });

    it('should call useGapiMutation with correct arguments', () => {
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue({
            dashboardState: sampleDashboardState,
          }),
        }),
      );
      renderHook(() => useGenerateDashboardState(), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiMutation).toHaveBeenCalledTimes(1);

      const requestBuilder = mockedUseGapiMutation.mock.calls[0][0];
      const builtRequest = requestBuilder(request);

      expect(builtRequest).toEqual({
        path: `${BASE_PATH}:generateDashboardState`,
        method: 'POST',
        body: request,
      });
    });

    it('should pass through options', () => {
      mockedUseGapiMutation.mockReturnValue(
        createMockMutationResult({
          mutateAsync: jest.fn().mockResolvedValue({
            dashboardState: sampleDashboardState,
          }),
        }),
      );
      const options = { mutationKey: ['test'] };
      renderHook(() => useGenerateDashboardState(options), {
        wrapper: FakeContextProvider,
      });
      expect(mockedUseGapiMutation).toHaveBeenCalledWith(
        expect.any(Function),
        options,
      );
    });
  });
});
