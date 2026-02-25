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
import { API_BASE_URL } from '@/crystal_ball/constants';
import {
  ListMeasurementFilterColumnsRequest,
  ListMeasurementFilterColumnsResponse,
  SuggestMeasurementFilterValuesRequest,
  SuggestMeasurementFilterValuesResponse,
} from '@/crystal_ball/types';

const BASE_PATH = `${API_BASE_URL}/v1`;

/**
 * Hook for ListMeasurementFilterColumns.
 * @param request - The list request payload.
 * @param options - Optional query options.
 * @returns A list of filterable columns.
 */
export const useListMeasurementFilterColumns = (
  request: ListMeasurementFilterColumnsRequest,
  options?: WrapperQueryOptions<ListMeasurementFilterColumnsResponse>,
): UseQueryResult<ListMeasurementFilterColumnsResponse> => {
  return useGapiQuery<ListMeasurementFilterColumnsResponse>(
    {
      path: `${BASE_PATH}/${request.parent}/measurementFilterColumns`,
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
 * Hook for SuggestMeasurementFilterValues.
 * @param request - The suggest request payload.
 * @param options - Optional query options.
 * @returns A list of suggested filter values.
 */
export const useSuggestMeasurementFilterValues = (
  request: SuggestMeasurementFilterValuesRequest,
  options?: WrapperQueryOptions<SuggestMeasurementFilterValuesResponse>,
): UseQueryResult<SuggestMeasurementFilterValuesResponse> => {
  return useGapiQuery<SuggestMeasurementFilterValuesResponse>(
    {
      path: `${BASE_PATH}/${request.parent}/measurementFilterColumns:suggestValues`,
      method: 'GET',
      params: {
        column: request.column,
        query: request.query,
        maxResultCount: request.maxResultCount,
      },
    },
    options,
  );
};
