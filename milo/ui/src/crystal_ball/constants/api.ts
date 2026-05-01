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

/**
 * Supported columns for querying and filtering.
 */
export enum Column {
  ATP_TEST_NAME = 'atp_test_name',
  BOARD = 'board',
  BUILD_BRANCH = 'build_branch',
  BUILD_CREATION_TIMESTAMP = 'build_creation_timestamp',
  BUILD_ID = 'build_id',
  BUILD_TARGET = 'build_target',
  BUILD_TYPE = 'build_type',
  INVOCATION_COMPLETE_TIMESTAMP = 'invocation_complete_timestamp',
  INVOCATION_URL = 'invocation_url',
  METRIC_KEY = 'metric_key',
  MODEL = 'model',
  PERFETTO_ARTIFACT_URL = 'perfetto_artifact_url',
  SKU = 'sku',
  TEST_NAME = 'test_name',
  VALUE = 'value',
}

/**
 * API Configuration.
 */
export const API_BASE_URL = 'https://crystalballperf.clients6.google.com';
export const API_V1_BASE_PATH = `${API_BASE_URL}/v1`;

/**
 * Delay in milliseconds for debouncing autocomplete queries.
 */
export const AUTOCOMPLETE_DEBOUNCE_DELAY_MS = 500;

/**
 * Default data spec identifier.
 */
export const DATA_SPEC_ID = 'cbdb';

/**
 * The column name that corresponds to the global time range filter.
 */
export const GLOBAL_TIME_RANGE_COLUMN = 'build_creation_timestamp';

/**
 * The filter ID for specifying the global time range.
 */
export const GLOBAL_TIME_RANGE_FILTER_ID = 'global_time_range';

/**
 * Default global time range option.
 */
export const GLOBAL_TIME_RANGE_OPTION_DEFAULT = '7d';

/**
 * Maximum page size allowed by the API.
 */
export const MAX_PAGE_SIZE = 1000;

/**
 * Maximum number of suggestions to return for autocomplete.
 */
export const MAX_SUGGEST_RESULTS = 10;

/**
 * Key for the number of aggregated rows in chart data points.
 */
export const NUM_AGGREGATED_ROWS = 'num_aggregated_rows';
