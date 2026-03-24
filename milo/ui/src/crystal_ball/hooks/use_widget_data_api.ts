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
import { API_V1_BASE_PATH as BASE_PATH } from '@/crystal_ball/constants';
import {
  FetchDashboardWidgetDataRequest,
  FetchDashboardWidgetDataResponse,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Hook for FetchDashboardWidgetData.
 * Uses a stub REST path as the exact service endpoint mapping is unknown.
 * @param request - The fetch request payload containing widget and data spec.
 * @param options - Optional query options.
 * @returns The dashboard widget data response.
 */
export const useFetchDashboardWidgetData = (
  request: FetchDashboardWidgetDataRequest,
  options?: WrapperQueryOptions<FetchDashboardWidgetDataResponse>,
): UseQueryResult<FetchDashboardWidgetDataResponse> => {
  const path = request.name
    ? `${BASE_PATH}/${request.name}:fetchDashboardWidgetData`
    : `${BASE_PATH}/dashboardStates:fetchDashboardWidgetData`;

  const { name: _name, ...body } = request;

  return useGapiQuery<FetchDashboardWidgetDataResponse>(
    {
      path,
      method: 'POST',
      body,
    },
    options,
  );
};
