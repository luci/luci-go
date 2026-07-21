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

import { useGapiQuery } from '@/common/hooks/gapi_query/gapi_query';
import { WrapperQueryOptions } from '@/common/types/query_wrapper_options';
import { API_V1_BASE_PATH } from '@/crystal_ball/constants';
import {
  ListAnnouncementsRequest,
  ListAnnouncementsResponse,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Returns the query key for ListAnnouncements.
 * Useful for cache invalidation.
 */
export const getListAnnouncementsQueryKey = (
  request?: ListAnnouncementsRequest,
) => {
  const params: Record<string, string | number> = {};
  if (request?.filter) {
    params.filter = request.filter;
  }
  if (request?.pageSize) {
    params.pageSize = request.pageSize;
  }
  if (request?.pageToken) {
    params.pageToken = request.pageToken;
  }
  return [
    'gapi',
    'GET',
    `${API_V1_BASE_PATH}/announcements`,
    Object.keys(params).length > 0 ? params : undefined,
    undefined,
  ];
};

/**
 * Hook for ListAnnouncements.
 * @param request - The list announcements request payload.
 * @param options - Optional query options.
 * @returns The ListAnnouncementsResponse.
 */
export const useListAnnouncements = (
  request?: ListAnnouncementsRequest,
  options?: WrapperQueryOptions<ListAnnouncementsResponse>,
): UseQueryResult<ListAnnouncementsResponse> => {
  const params: Record<string, string | number> = {};
  if (request?.filter) {
    params.filter = request.filter;
  }
  if (request?.pageSize) {
    params.pageSize = request.pageSize;
  }
  if (request?.pageToken) {
    params.pageToken = request.pageToken;
  }

  return useGapiQuery<ListAnnouncementsResponse>(
    {
      path: `${API_V1_BASE_PATH}/announcements`,
      method: 'GET',
      params: Object.keys(params).length > 0 ? params : undefined,
    },
    options,
  );
};
