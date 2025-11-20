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

export const MAXIMUM_PAGE_SIZE = 1000;

/**
 * google.protobuf.Timestamp
 */
export interface Timestamp {
  /**
   * Represents seconds of UTC time since Unix epoch.
   */
  seconds?: number;

  /**
   * Non-negative fractions of a second at nanosecond resolution.
   */
  nanos?: number;
}

/**
 * google.protobuf.Value
 */
type Value = null | number | string | boolean | Struct | Array<Value>;

/**
 * google.protobuf.Struct
 */
interface Struct {
  [key: string]: Value;
}

/**
 * Request message for querying CrystalBall measurements.
 * Requests must include at least one filter or the request will be rejected.
 */
export interface SearchMeasurementsRequest {
  /**
   * Filter results to a specified test name.
   * e.g. "ExampleGroup.ExampleSubGroup#ExampleTestName"
   */
  testNameFilter?: string;

  /**
   * Filter results to build create times greater than this value.
   * If "lastNDays" is provided, this filter will be ignored.
   */
  buildCreateStartTime?: Timestamp;

  /**
   * Filter results to build create times less than this value.
   * If "lastNDays" is provided, this filter will be ignored.
   */
  buildCreateEndTime?: Timestamp;

  /**
   * Filter results to build create times within the last N days.
   */
  lastNDays?: number;

  /**
   * Filter results to a specified build branch.
   * e.g. "example_git_main"
   */
  buildBranch?: string;

  /**
   * Filter results to a specified build target.
   * e.g. "example-build-target"
   */
  buildTarget?: string;

  /**
   * Filter results to a specified ATP test name.
   * e.g. "v2/example-test-group/example-test-name"
   */
  atpTestNameFilter?: string;

  /**
   * Filter results to only include the specified metric keys.
   * e.g. ["sample-metric-key-A", "sample-metric-key-B"]
   */
  metricKeys?: string[];

  /**
   * Include additional columns in the query to be returned by the response.
   * In order to be valid, columns must exist and not match the above filter
   * columns.
   * e.g. ["board", "model"]
   */
  extraColumns?: string[];

  /**
   * Optional specifier for the amount of rows to include in the response.
   * Default is 100.
   * Max is 1,000.
   */
  pageSize?: number;

  /**
   * Optional specifier for a page token when paginating results.
   * If provided, will grab results for the page as determined by the current
   * query offset.
   */
  pageToken?: string;
}

/**
 * Response message when querying CrystalBall measurements.
 */
export interface SearchMeasurementsResponse {
  /**
   * List of results rows.
   */
  rows: MeasurementRow[];

  /**
   * If the count of rows exceeds "pageSize", then a token will be provided for
   * querying the next page of results.
   */
  nextPageToken: string;
}

/**
 * Row message for holding the query results.
 */
export interface MeasurementRow {
  /**
   * Test name of the measurement.
   * e.g. "ExampleGroup.ExampleSubGroup#ExampleTestName"
   */
  test?: string;

  /**
   * Build create time of the measurement.
   */
  buildCreateTime?: string;

  /**
   * Build branch of the measurement.
   * e.g. "example_git_main"
   */
  buildBranch?: string;

  /**
   * Build target of the measurement.
   * e.g. "example-build-target"
   */
  buildTarget?: string;

  /**
   * ATP Test Name of the measurement.
   * e.g. "v2/example-test-group/example-test-name"
   */
  atpTest?: string;

  /**
   * Metric Key of the measurement.
   * e.g. ["sample-metric-key-A", "sample-metric-key-B"]
   */
  metricKey?: string;

  /**
   * Value of the measurement for plotting on the y-axis of a time series chart.
   * e.g. 92.0
   */
  value?: number;

  /**
   * Build id of the measurement.
   * e.g. 12345678
   */
  buildId?: string;

  /**
   * AnTS invocation id of the measurement.
   * e.g. "I12300012398798712"
   */
  antsInvocationId?: string;

  /**
   * If the request included one or more extra columns, their values will be
   * included here as a map with their column name corresponding to their
   * underlying value.
   * e.g.
   * {
   *    "board": "example-board",
   *    "model": "example-model",
   * }
   */
  extraColumns?: { [key: string]: Value };
}

/**
 * API Configuration.
 */
export const API_BASE_URL = 'https://crystalballperf.clients6.google.com';

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
