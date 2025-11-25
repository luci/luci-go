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

import {
  InfiniteData,
  UseInfiniteQueryResult,
  UseQueryResult,
} from '@tanstack/react-query';

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
  SearchMeasurementsRequest,
  SearchMeasurementsResponse,
} from '@/crystal_ball/types';

/**
 * Hook for TestConnection.
 * @param options - optional query options.
 * @returns an empty response if successful.
 */
export const useTestConnection = (
  options?: WrapperQueryOptions<object>,
): UseQueryResult<object> => {
  return useGapiQuery<object>(
    {
      path: `${API_BASE_URL}/v1/perf:testConnection`,
      method: 'GET',
      params: {},
    },
    options,
  );
};

/**
 * Hook for SearchMeasurements (standard query).
 * @param request - search measurements request payload.
 * @param options - optional query options.
 * @returns a search measurements response.
 */
export const useSearchMeasurements = (
  request: SearchMeasurementsRequest,
  options?: WrapperQueryOptions<SearchMeasurementsResponse>,
): UseQueryResult<SearchMeasurementsResponse> => {
  return useGapiQuery<SearchMeasurementsResponse>(
    {
      path: `${API_BASE_URL}/v1/measurements:search`,
      method: 'GET',
      params: request,
    },
    options,
  );
};

/**
 * Hook for SearchMeasurements (infinite query version).
 * @param request - search measurements request payload.
 * @param options - optional query options.
 * @returns an infinite query result for search measurements.
 */
export const useSearchMeasurementsInfinite = (
  request: Omit<SearchMeasurementsRequest, 'pageToken'>,
  options?: WrapperInfiniteQueryOptions<SearchMeasurementsResponse>,
): UseInfiniteQueryResult<InfiniteData<SearchMeasurementsResponse>, Error> => {
  return useInfiniteGapiQuery<SearchMeasurementsResponse>(
    {
      path: `${API_BASE_URL}/v1/measurements:search`,
      method: 'GET',
      params: request,
    },
    options,
  );
};
