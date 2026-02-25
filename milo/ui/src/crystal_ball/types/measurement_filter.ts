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

/**
 * Request message for getting filter columns.
 */
export interface ListMeasurementFilterColumnsRequest {
  /**
   * The parent PerfDataSpec resource name.
   * Format: dashboardStates/{dashboard_state}/dataSpecs/{data_spec}
   */
  parent: string;

  /**
   * The maximum number of columns to return.
   */
  pageSize?: number;

  /**
   * A page token, received from a previous call.
   */
  pageToken?: string;
}

/**
 * Response message for getting filter columns.
 */
export interface ListMeasurementFilterColumnsResponse {
  /**
   * The filter columns.
   */
  measurementFilterColumns?: MeasurementFilterColumn[];

  /**
   * A token to retrieve the next page.
   */
  nextPageToken?: string;
}

/**
 * Represents a filterable column for measurements.
 */
export interface MeasurementFilterColumn {
  /**
   * The unique identifier for the column within the data spec.
   */
  column?: string;

  /**
   * The data type of the column.
   */
  dataType?: ColumnDataType;

  /**
   * A small list of example values for this column.
   */
  sampleValues?: string[];

  /**
   * Indicates if this is a primary column.
   */
  primary?: boolean;

  /**
   * Indicates if this column represents a plottable metric key.
   */
  isMetricKey?: boolean;

  /**
   * The scopes at which this column can be applied as a filter.
   */
  applicableScopes?: FilterScope[];
}

/**
 * Enum defining the possible data types for a filter column.
 */
export enum ColumnDataType {
  COLUMN_DATA_TYPE_UNSPECIFIED = 'COLUMN_DATA_TYPE_UNSPECIFIED',
  STRING = 'STRING',
  INT64 = 'INT64',
  DOUBLE = 'DOUBLE',
  BOOLEAN = 'BOOLEAN',
  TIMESTAMP = 'TIMESTAMP',
  DATE = 'DATE',
}

/**
 * Defines the possible scopes where a filter column can be applied.
 */
export enum FilterScope {
  FILTER_SCOPE_UNSPECIFIED = 'FILTER_SCOPE_UNSPECIFIED',
  GLOBAL = 'GLOBAL',
  WIDGET = 'WIDGET',
  METRIC = 'METRIC',
}

/**
 * Request message for getting filter values.
 */
export interface SuggestMeasurementFilterValuesRequest {
  /**
   * The parent PerfDataSpec resource name.
   * Format: dashboardStates/{dashboard_state}/dataSpecs/{data_spec}
   */
  parent: string;

  /**
   * The column name to get values from.
   */
  column: string;

  /**
   * The prefix string the user has typed for autocomplete.
   */
  query?: string;

  /**
   * Maximum number of suggestions to return.
   */
  maxResultCount?: number;
}

/**
 * Response message for getting filter values.
 */
export interface SuggestMeasurementFilterValuesResponse {
  /**
   * The list of filter values.
   */
  values?: string[];
}
