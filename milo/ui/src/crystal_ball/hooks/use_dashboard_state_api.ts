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

import { UseQueryResult } from '@tanstack/react-query';

import {
  useGapiQuery,
  useInfiniteGapiQuery,
} from '@/common/hooks/gapi_query/gapi_query';
import {
  WrapperInfiniteQueryOptions,
  WrapperQueryOptions,
} from '@/common/types/query_wrapper_options';
import { API_BASE_URL } from '@/crystal_ball/constants';
import {
  CreateDashboardStateRequest,
  DashboardState,
  DashboardStateOperation,
  DeleteDashboardStateRequest,
  GetDashboardStateRequest,
  GetDashboardStateRevisionRequest,
  ListDashboardStateRevisionsRequest,
  ListDashboardStateRevisionsResponse,
  ListDashboardStatesRequest,
  ListDashboardStatesResponse,
  RollbackDashboardStateRequest,
  UndeleteDashboardStateRequest,
  UpdateDashboardStateRequest,
} from '@/crystal_ball/types';

const BASE_PATH = `${API_BASE_URL}/v1`;

/**
 * Hook for CreateDashboardState.
 * @param request - The create request payload.
 * @param options - Optional query options.
 * @returns A Long Running Operation.
 */
export const useCreateDashboardState = (
  request: CreateDashboardStateRequest,
  options?: WrapperQueryOptions<DashboardStateOperation>,
): UseQueryResult<DashboardStateOperation> => {
  return useGapiQuery<DashboardStateOperation>(
    {
      path: `${BASE_PATH}/dashboardStates`,
      method: 'POST',
      body: request.dashboardState,
      params: {
        dashboardStateId: request.dashboardStateId,
        validateOnly: request.validateOnly,
        requestId: request.requestId,
      },
    },
    options,
  );
};

/**
 * Hook for GetDashboardState.
 * @param request - The get request payload.
 * @param options - Optional query options.
 * @returns The DashboardState.
 */
export const useGetDashboardState = (
  request: GetDashboardStateRequest,
  options?: WrapperQueryOptions<DashboardState>,
): UseQueryResult<DashboardState> => {
  return useGapiQuery<DashboardState>(
    {
      path: `${BASE_PATH}/${request.name}`,
      method: 'GET',
    },
    options,
  );
};

/**
 * Hook for ListDashboardStates.
 * @param request - The list request payload.
 * @param options - Optional query options.
 * @returns A list of DashboardStates.
 */
export const useListDashboardStates = (
  request: ListDashboardStatesRequest,
  options?: WrapperQueryOptions<ListDashboardStatesResponse>,
): UseQueryResult<ListDashboardStatesResponse> => {
  return useGapiQuery<ListDashboardStatesResponse>(
    {
      path: `${BASE_PATH}/dashboardStates`,
      method: 'GET',
      params: request,
    },
    options,
  );
};

/**
 * Hook for ListDashboardStates (infinite query version).
 * @param request - The list request payload without pageToken.
 * @param options - Optional query options.
 * @returns An infinite query result for DashboardStates.
 */
export const useListDashboardStatesInfinite = (
  request: Omit<ListDashboardStatesRequest, 'pageToken'>,
  options?: WrapperInfiniteQueryOptions<ListDashboardStatesResponse>,
) => {
  return useInfiniteGapiQuery<ListDashboardStatesResponse>(
    {
      path: `${BASE_PATH}/dashboardStates`,
      method: 'GET',
      params: request,
    },
    options,
  );
};

/**
 * Hook for UpdateDashboardState.
 * @param request - The update request payload.
 * @param options - Optional query options.
 * @returns A Long Running Operation.
 */
export const useUpdateDashboardState = (
  request: UpdateDashboardStateRequest,
  options?: WrapperQueryOptions<DashboardStateOperation>,
): UseQueryResult<DashboardStateOperation> => {
  return useGapiQuery<DashboardStateOperation>(
    {
      path: `${BASE_PATH}/${request.dashboardState?.name}`,
      method: 'PATCH',
      body: request.dashboardState,
      params: {
        updateMask: request.updateMask?.paths?.join(','),
        allowMissing: request.allowMissing,
        validateOnly: request.validateOnly,
        requestId: request.requestId,
      },
    },
    options,
  );
};

/**
 * Hook for DeleteDashboardState.
 * @param request - The delete request payload.
 * @param options - Optional query options.
 * @returns A Long Running Operation.
 */
export const useDeleteDashboardState = (
  request: DeleteDashboardStateRequest,
  options?: WrapperQueryOptions<DashboardStateOperation>,
): UseQueryResult<DashboardStateOperation> => {
  return useGapiQuery<DashboardStateOperation>(
    {
      path: `${BASE_PATH}/${request.name}`,
      method: 'DELETE',
      params: {
        etag: request.etag,
        validateOnly: request.validateOnly,
        allowMissing: request.allowMissing,
        requestId: request.requestId,
      },
    },
    options,
  );
};

/**
 * Hook for UndeleteDashboardState.
 * @param request - The undelete request payload.
 * @param options - Optional query options.
 * @returns A Long Running Operation.
 */
export const useUndeleteDashboardState = (
  request: UndeleteDashboardStateRequest,
  options?: WrapperQueryOptions<DashboardStateOperation>,
): UseQueryResult<DashboardStateOperation> => {
  return useGapiQuery<DashboardStateOperation>(
    {
      path: `${BASE_PATH}/${request.name}:undelete`,
      method: 'POST',
      body: request, // Body includes etag, requestId, validateOnly
    },
    options,
  );
};

/**
 * Hook for ListDashboardStateRevisions.
 * @param request - The list revisions request payload.
 * @param options - Optional query options.
 * @returns A list of DashboardState revisions.
 */
export const useListDashboardStateRevisions = (
  request: ListDashboardStateRevisionsRequest,
  options?: WrapperQueryOptions<ListDashboardStateRevisionsResponse>,
): UseQueryResult<ListDashboardStateRevisionsResponse> => {
  return useGapiQuery<ListDashboardStateRevisionsResponse>(
    {
      path: `${BASE_PATH}/${request.name}/revisions`,
      method: 'GET',
      params: {
        pageSize: request.pageSize,
        pageToken: request.pageToken,
      },
    },
    options,
  );
};

/**
 * Hook for GetDashboardStateRevision.
 * @param request - The get revision request payload.
 * @param options - Optional query options.
 * @returns The DashboardState representing the revision.
 */
export const useGetDashboardStateRevision = (
  request: GetDashboardStateRevisionRequest,
  options?: WrapperQueryOptions<DashboardState>,
): UseQueryResult<DashboardState> => {
  return useGapiQuery<DashboardState>(
    {
      path: `${BASE_PATH}/${request.name}`,
      method: 'GET',
    },
    options,
  );
};

/**
 * Hook for RollbackDashboardState.
 * @param request - The rollback request payload.
 * @param options - Optional query options.
 * @returns The DashboardState after rollback.
 */
export const useRollbackDashboardState = (
  request: RollbackDashboardStateRequest,
  options?: WrapperQueryOptions<DashboardState>,
): UseQueryResult<DashboardState> => {
  return useGapiQuery<DashboardState>(
    {
      path: `${BASE_PATH}/${request.name}:rollback`,
      method: 'POST',
      body: request,
    },
    options,
  );
};
