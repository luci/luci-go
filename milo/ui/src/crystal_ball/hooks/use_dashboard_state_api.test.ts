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
import { API_BASE_URL } from '@/crystal_ball/constants';
import {
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
} from '@/crystal_ball/hooks';
import { TypedDashboardStateOperation } from '@/crystal_ball/hooks/use_dashboard_state_api';
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
} from '@/crystal_ball/types';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

// Mock the imported hooks
jest.mock('@/common/hooks/gapi_query/gapi_query', () => ({
  useGapiQuery: jest.fn(),
  useInfiniteGapiQuery: jest.fn(),
  useGapiMutation: jest.fn(),
}));

const mockedUseGapiQuery = gapiQueryHooks.useGapiQuery as jest.Mock;
const mockedUseInfiniteGapiQuery =
  gapiQueryHooks.useInfiniteGapiQuery as jest.Mock;
const mockedUseGapiMutation = gapiQueryHooks.useGapiMutation as jest.Mock;

const BASE_PATH = `${API_BASE_URL}/v1`;

describe('use_dashboard_state_api', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  // Example minimal DashboardState for testing
  const sampleDashboardState: DashboardState = {
    name: 'dashboardStates/test-id',
    displayName: 'Test Dashboard',
    dashboardContent: {}, // Add content as needed
  };

  const sampleOperation: TypedDashboardStateOperation = {
    name: 'operations/test-operation-id',
    done: true,
    response: sampleDashboardState,
  };

  describe('useCreateDashboardState', () => {
    const request: CreateDashboardStateRequest = {
      dashboardState: { dashboardContent: {}, displayName: 'New Dash' },
      dashboardStateId: 'new-dash-id',
    };

    it('should call useGapiMutation with correct arguments', () => {
      // Mock mutateAsync to resolve with our strongly typed operation
      mockedUseGapiMutation.mockReturnValue({
        mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        isPending: false,
      });
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
          validateOnly: undefined,
          requestId: undefined,
        },
      });
    });

    it('should pass through options', () => {
      mockedUseGapiMutation.mockReturnValue({
        mutateAsync: jest.fn().mockResolvedValue(sampleOperation),
        isPending: false,
      });
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
    const request: GetDashboardStateRequest = {
      name: 'dashboardStates/test-id',
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: sampleDashboardState,
        isLoading: false,
      });
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
      mockedUseGapiQuery.mockReturnValue({
        data: sampleDashboardState,
        isLoading: false,
        isSuccess: true,
      });
      const { result } = renderHook(() => useGetDashboardState(request), {
        wrapper: FakeContextProvider,
      });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual(sampleDashboardState);
    });
  });

  describe('useListDashboardStates', () => {
    const request: ListDashboardStatesRequest = { pageSize: 10 };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: { dashboardStates: [sampleDashboardState] },
        isLoading: false,
      });
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
    const request: Omit<ListDashboardStatesRequest, 'pageToken'> = {
      pageSize: 20,
    };

    it('should call useInfiniteGapiQuery with correct arguments', () => {
      mockedUseInfiniteGapiQuery.mockReturnValue({
        data: {
          pages: [{ dashboardStates: [sampleDashboardState] }],
          pageParams: [],
        },
        isLoading: false,
        fetchNextPage: jest.fn(),
      });

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
      mockedUseInfiniteGapiQuery.mockReturnValue({
        data: { pages: [], pageParams: [] },
        isLoading: false,
        fetchNextPage: jest.fn(),
      });
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
    const request: UpdateDashboardStateRequest = {
      dashboardState: sampleDashboardState,
      updateMask: { paths: ['displayName'] },
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: sampleOperation,
        isLoading: false,
      });
      renderHook(() => useUpdateDashboardState(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${sampleDashboardState.name}`,
          method: 'PATCH',
          body: request.dashboardState,
          params: {
            updateMask: 'displayName',
            allowMissing: undefined,
            validateOnly: undefined,
            requestId: undefined,
          },
        },
        undefined,
      );
    });
  });

  describe('useDeleteDashboardState', () => {
    const request: DeleteDashboardStateRequest = {
      name: 'dashboardStates/test-id',
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: sampleOperation,
        isLoading: false,
      });
      renderHook(() => useDeleteDashboardState(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.name}`,
          method: 'DELETE',
          params: {
            etag: undefined,
            validateOnly: undefined,
            allowMissing: undefined,
            requestId: undefined,
          },
        },
        undefined,
      );
    });
  });

  describe('useUndeleteDashboardState', () => {
    const request: UndeleteDashboardStateRequest = {
      name: 'dashboardStates/test-id',
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: sampleOperation,
        isLoading: false,
      });
      renderHook(() => useUndeleteDashboardState(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.name}:undelete`,
          method: 'POST',
          body: request,
        },
        undefined,
      );
    });
  });

  describe('useListDashboardStateRevisions', () => {
    const request: ListDashboardStateRevisionsRequest = {
      name: 'dashboardStates/test-id',
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: { dashboardStates: [sampleDashboardState] },
        isLoading: false,
      });
      renderHook(() => useListDashboardStateRevisions(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.name}/revisions`,
          method: 'GET',
          params: {
            pageSize: undefined,
            pageToken: undefined,
          },
        },
        undefined,
      );
    });
  });

  describe('useGetDashboardStateRevision', () => {
    const request: GetDashboardStateRevisionRequest = {
      name: 'dashboardStates/test-id/revisions/rev1',
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: sampleDashboardState,
        isLoading: false,
      });
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
    const request: RollbackDashboardStateRequest = {
      name: 'dashboardStates/test-id',
      revisionId: 'rev1',
    };

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: sampleDashboardState,
        isLoading: false,
      });
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
});
