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

import { Timestamp, Value } from '@/crystal_ball/types';

/**
 * Possible filter names for the SearchMeasurementsRequest.
 */
export enum SearchMeasurementsFilter {
  TEST = 'testNameFilter',
  BUILD_CREATE_START_TIME = 'buildCreateStartTime',
  BUILD_CREATE_END_TIME = 'buildCreateEndTime',
  LAST_N_DAYS = 'lastNDays',
  BUILD_BRANCH = 'buildBranch',
  BUILD_TARGET = 'buildTarget',
  ATP_TEST = 'atpTestNameFilter',
  METRIC_KEYS = 'metricKeys',
  EXTRA_COLUMNS = 'extraColumns',
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
  [SearchMeasurementsFilter.TEST]?: string;

  /**
   * Filter results to build create times greater than this value.
   * If "lastNDays" is provided, this filter will be ignored.
   */
  [SearchMeasurementsFilter.BUILD_CREATE_START_TIME]?: Timestamp;

  /**
   * Filter results to build create times less than this value.
   * If "lastNDays" is provided, this filter will be ignored.
   */
  [SearchMeasurementsFilter.BUILD_CREATE_END_TIME]?: Timestamp;

  /**
   * Filter results to build create times within the last N days.
   */
  [SearchMeasurementsFilter.LAST_N_DAYS]?: number;

  /**
   * Filter results to a specified build branch.
   * e.g. "example_git_main"
   */
  [SearchMeasurementsFilter.BUILD_BRANCH]?: string;

  /**
   * Filter results to a specified build target.
   * e.g. "example-build-target"
   */
  [SearchMeasurementsFilter.BUILD_TARGET]?: string;

  /**
   * Filter results to a specified ATP test name.
   * e.g. "v2/example-test-group/example-test-name"
   */
  [SearchMeasurementsFilter.ATP_TEST]?: string;

  /**
   * Filter results to only include the specified metric keys.
   * e.g. ["sample-metric-key-A", "sample-metric-key-B"]
   */
  [SearchMeasurementsFilter.METRIC_KEYS]?: string[];

  /**
   * Include additional columns in the query to be returned by the response.
   * In order to be valid, columns must exist and not match the above filter
   * columns.
   * e.g. ["board", "model"]
   */
  [SearchMeasurementsFilter.EXTRA_COLUMNS]?: string[];

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
