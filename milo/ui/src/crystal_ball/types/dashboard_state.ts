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

import { FieldMask, Operation, Timestamp } from './proto';

/**
 * google.android.perf.v1.PerfDashboardContent
 */
export interface PerfDashboardContent {
  /**
   * List of data specs (sources) associated with this dashboard state.
   */
  dataSpecs?: { [key: string]: PerfDataSpec };

  /**
   * List of global filters to be applied to this dashboard state.
   */
  globalFilters?: PerfFilter[];

  /**
   * List of widgets contained within this dashboard state.
   */
  widgets?: PerfWidget[];
}

/**
 * google.android.perf.v1.PerfDataSpec
 */
export interface PerfDataSpec {
  /**
   * Identifier for this data spec (source).
   */
  displayName?: string;

  /**
   * Details for this data spec.
   */
  source?: PerfDataSource;
}

/**
 * google.android.perf.v1.PerfDataSource
 */
export interface PerfDataSource {
  /**
   * Possible types for the data source.
   */
  type?: 'SOURCE_TYPE_UNSPECIFIED' | 'SQL' | 'TABLE';
}

/**
 * google.android.perf.v1.PerfFilter
 */
export interface PerfFilter {
  /**
   * Identifier for this filter.
   */
  id: string;

  /**
   * Column identifier for this filter.
   */
  column: string;

  /**
   * Data spec identifier for this filter.
   */
  dataSpecId: string;

  /**
   * Friendly name for this filter.
   */
  displayName?: string;

  /**
   * If toggled, contains the SelectFilter information.
   */
  select?: SelectFilter;

  /**
   * If toggled, contains the TextInputFilter information.
   */
  textInput?: TextInputFilter;

  /**
   * If toggled, contains the NumberInputFilter information.
   */
  numberInput?: NumberInputFilter;

  /**
   * If toggled, contains the RangeFilter information.
   */
  range?: RangeFilter;

  /**
   * If toggled, contains the DatePickerFilter information.
   */
  datePicker?: DatePickerFilter;
}

/**
 * google.android.perf.v1.SelectFilter
 */
export interface SelectFilter {
  /**
   * Controls whether this selection filter supports selecting multiple values.
   */
  multiSelect?: boolean;

  /**
   * Default settings for this filter.
   */
  defaultValue?: PerfFilterDefault;
}

/**
 * google.android.perf.v1.TextInputFilter
 */
export interface TextInputFilter {
  /**
   * Default settings for this filter.
   */
  defaultValue?: PerfFilterDefault;
}

/**
 * google.android.perf.v1.NumberInputFilter
 */
export interface NumberInputFilter {
  /**
   * Default settings for this filter.
   */
  defaultValue?: PerfFilterDefault;
}

/**
 * google.android.perf.v1.RangeFilter
 */
export interface RangeFilter {
  /**
   * Default settings for this filter.
   */
  defaultValue?: PerfFilterDefault;
}

/**
 * google.android.perf.v1.DatePickerFilter
 */
export interface DatePickerFilter {
  /**
   * Controls whether this selection filter supports selecting multiple values.
   */
  multiSelect?: boolean;

  /**
   * Default settings for this filter.
   */
  defaultValue?: PerfFilterDefault;
}

/**
 * google.android.perf.v1.PerfFilterDefault
 */
export interface PerfFilterDefault {
  /**
   * List of value selections associated with this filter.
   */
  values?: string[];

  /**
   * Operator to apply to this filter.
   */
  filterOperator?:
    | 'FILTER_OPERATOR_UNSPECIFIED'
    | 'EQUAL'
    | 'NOT_EQUAL'
    | 'GREATER_THAN'
    | 'LESS_THAN'
    | 'GREATER_THAN_OR_EQUAL'
    | 'LESS_THAN_OR_EQUAL'
    | 'IN'
    | 'NOT_IN'
    | 'BETWEEN'
    | 'REGEX_MATCH'
    | 'NOT_REGEX_MATCH'
    | 'IS_EMPTY'
    | 'IS_NOT_EMPTY'
    | 'STARTS_WITH'
    | 'NOT_STARTS_WITH'
    | 'ENDS_WITH'
    | 'NOT_ENDS_WITH'
    | 'CONTAINS'
    | 'NOT_CONTAINS'
    | 'LIKE'
    | 'NOT_LIKE'
    | 'IS_TRUE'
    | 'IS_FALSE'
    | 'IS_NULL'
    | 'IS_NOT_NULL';
}

/**
 * google.android.perf.v1.PerfWidget
 */
export interface PerfWidget {
  /**
   * Identifier for this widget.
   */
  id: string;

  /**
   * Friendly name for this widget.
   */
  displayName?: string;

  /**
   * If toggled, contains markdown information.
   */
  markdown?: MarkdownWidget;

  /**
   * If toggled, contains perf chart information.
   */
  chart?: PerfChartWidget;
}

/**
 * google.android.perf.v1.MarkdownWidget
 */
export interface MarkdownWidget {
  /**
   * Markdown text content.
   */
  content?: string;
}

/**
 * google.android.perf.v1.PerfChartWidget
 */
export interface PerfChartWidget {
  /**
   * Friendly name for the chart within this widget.
   */
  displayName?: string;

  /**
   * Chart type for this widget.
   */
  chartType?:
    | 'CHART_TYPE_UNSPECIFIED'
    | 'REGRESSION_METRIC_CHART'
    | 'MULTI_METRIC_CHART'
    | 'BREAKDOWN_TABLE'
    | 'INVOCATION_DISTRIBUTION';

  /**
   * Intermediate chart type state for this widget.
   */
  effectiveChartType?:
    | 'CHART_TYPE_UNSPECIFIED'
    | 'REGRESSION_METRIC_CHART'
    | 'MULTI_METRIC_CHART'
    | 'BREAKDOWN_TABLE'
    | 'INVOCATION_DISTRIBUTION';

  /**
   * Data spec to associate with this chart.
   */
  dataSpecId: string;

  /**
   * Chart level filters to apply to this chart.
   */
  filters?: PerfFilter[];

  /**
   * List of series to display on this chart.
   */
  series?: PerfChartSeries[];

  /**
   * X-axis configuration for this chart.
   */
  xAxis?: PerfXAxisConfig;

  /**
   * If toggled, contains left hand side y-axis information
   */
  leftYAxis?: PerfYAxisConfig;

  /**
   * If toggled, contains right hand side y-axis information
   */
  rightYAxis?: PerfYAxisConfig;

  /**
   * Details regarding series splits that have been invoked on this chart.
   */
  seriesSplit?: PerfSeriesSplit;

  /**
   * Invocation distribution configuration.
   */
  invocationDistributionConfig?: InvocationDistributionConfig;
}

/**
 * google.android.perf.v1.PerfChartSeries
 */
export interface PerfChartSeries {
  /**
   * Friendly name for this series.
   */
  displayName?: string;

  /**
   * Identifier for the data spec associated with this series.
   */
  dataSpecId?: string;

  /**
   * Metric key identifier for this series.
   */
  metricField: string;

  /**
   * Metric level filters to apply to this series.
   */
  filters?: PerfFilter[];

  /**
   * The type of aggregation applied to this series.
   */
  aggregation?:
    | 'PERF_AGGREGATION_FUNCTION_UNSPECIFIED'
    | 'MEAN'
    | 'P50'
    | 'P75'
    | 'P90'
    | 'P99'
    | 'MIN'
    | 'MAX'
    | 'COUNT';

  /**
   * Indicator for y-axis assignment on this series.
   */
  yAxisAssignment?: 'Y_AXIS_ASSIGNMENT_UNSPECIFIED' | 'LEFT' | 'RIGHT';

  /**
   * Flag to control confidence interval display.
   */
  showConfidenceInterval?: boolean;

  /**
   * Intermediate data spec id display.
   */
  effectiveDataSpecId?: string;

  /**
   * Intermediate y-axis display configuration.
   */
  effectiveYAxisAssignment?: 'Y_AXIS_ASSIGNMENT_UNSPECIFIED' | 'LEFT' | 'RIGHT';

  /**
   * Color selected for this chart series.
   */
  color?: string;
}

/**
 * google.android.perf.v1.PerfXAxisConfig
 */
export interface PerfXAxisConfig {
  /**
   * Column from the data spec associated with this x-axis.
   */
  column: string;

  /**
   * Granularity to apply to this x-axis.
   */
  granularity?:
    | 'GRANULARITY_UNSPECIFIED'
    | 'PER_VALUE'
    | 'HOURLY'
    | 'DAILY'
    | 'WEEKLY'
    | 'PER_BUILD';

  /**
   * Intermediate state for the granularity.
   */
  effectiveGranularity?:
    | 'GRANULARITY_UNSPECIFIED'
    | 'PER_VALUE'
    | 'HOURLY'
    | 'DAILY'
    | 'WEEKLY'
    | 'PER_BUILD';
}

/**
 * google.android.perf.v1.PerfYAxisConfig
 */
export interface PerfYAxisConfig {
  /**
   * Friendly name for this y-axis configuration.
   */
  displayName?: string;

  /**
   * Minimum value for the axis.
   */
  minValue?: number;

  /**
   * Maximum value for the axis.
   */
  maxValue?: number;

  /**
   * Flag to control a logarithmic scale.
   */
  logarithmic?: boolean;
}

/**
 * google.android.perf.v1.PerfSeriesSplit
 */
export interface PerfSeriesSplit {
  /**
   * Dimension identifier associated with this series split.
   */
  invocationDimension?: string;

  /**
   * Metric dimension identifier associated with this series split.
   */
  metricDimension?: string;

  /**
   * Limit count for this split.
   */
  limitCount?: number;
}

/**
 * google.android.perf.v1.InvocationDistributionConfig
 */
export interface InvocationDistributionConfig {
  /**
   * Scatter plot settings.
   */
  scatterSettings?: {
    /**
     * Metric field to associate with the x-axis.
     */
    xAxisMetricField?: string;
  };
}

/**
 * google.android.perf.v1.DashboardState
 */
export interface DashboardState {
  /**
   * Identifier for this dashboard state.
   */
  name?: string;

  /**
   * Content of this dashboard state.
   */
  dashboardContent: PerfDashboardContent;

  /**
   * Friendly name for this dashboard state.
   */
  displayName?: string;

  /**
   * Description for this dashboard state.
   */
  description?: string;

  /**
   * Create time for this dashboard state.
   */
  createTime?: Timestamp;

  /**
   * Last updated time for this dashboard state.
   */
  updateTime?: Timestamp;

  /**
   * If deleted, records the deleted time for this dashboard state.
   */
  deleteTime?: Timestamp;

  /**
   * If deleted, records the time this dashboard state will be purged.
   */
  purgeTime?: Timestamp;

  /**
   * Identifier to guard against concurrent overwriting.
   */
  etag?: string;

  /**
   * Server generated identifier for this dashboard state.
   */
  uid?: string;

  /**
   * Annotations applied to this dashboard state.
   */
  annotations?: { [key: string]: string };

  /**
   * Flag to indicate whether the dashboard is currently undergoing reconciliation.
   */
  reconciling?: boolean;

  /**
   * Latest revision identifier for this dashboard state.
   */
  revisionId?: string;

  /**
   * Create time of the latest revision associated with this dashboard state.
   */
  revisionCreateTime?: Timestamp;
}

/**
 * google.android.perf.v1.DashboardStateOperationMetadata
 */
export interface DashboardStateOperationMetadata {
  /**
   * The time at which this operation was created.
   */
  createTime?: Timestamp;

  /**
   * The dashboard state resource identifier that this operation corresponds to.
   */
  target?: string;

  /**
   * The version of the API used in this operation context.
   */
  apiVersion?: string;
}

/**
 * google.android.perf.v1.DashboardStateOperation
 */
export type DashboardStateOperation = Operation<
  DashboardStateOperationMetadata,
  DashboardState,
  string
>;

/**
 * google.android.perf.v1.CreateDashboardStateRequest
 */
export interface CreateDashboardStateRequest {
  /**
   * Dashboard state content.
   */
  dashboardState: DashboardState;

  /**
   * Optional identifier for the dashboard state.
   * If not specified, the server will generate one.
   */
  dashboardStateId?: string;

  /**
   * If true, the server will only check if the operation is valid.
   */
  validateOnly?: boolean;

  /**
   * Identifier for telemetry.
   */
  requestId?: string;
}

/**
 * google.android.perf.v1.UpdateDashboardStateRequest
 */
export interface UpdateDashboardStateRequest {
  /**
   * Dashboard state content.
   */
  dashboardState: DashboardState;

  /**
   * Fields to update during this update request.
   */
  updateMask?: FieldMask;

  /**
   * Whether or not to allow missing properties which will be ignored.
   */
  allowMissing?: boolean;

  /**
   * If true, the server will only check if the operation is valid.
   */
  validateOnly?: boolean;

  /**
   * Identifier for telemetry.
   */
  requestId?: string;
}

/**
 * google.android.perf.v1.GetDashboardStateRequest
 */
export interface GetDashboardStateRequest {
  /**
   * Identifier for the dashboard state to retrieve.
   */
  name: string;
}

/**
 * google.android.perf.v1.ListDashboardStatesRequest
 */
export interface ListDashboardStatesRequest {
  /**
   * If set, determines the amount of dashboard states to return per page.
   */
  pageSize?: number;

  /**
   * If set, grabs results for the next page (from a previously paged response).
   */
  pageToken?: string;

  /**
   * String filter to apply to dashboard state identifiers.
   */
  filter?: string;

  /**
   * Whether or not to show deleted dashboard states.
   */
  showDeleted?: boolean;
}

/**
 * google.android.perf.v1.ListDashboardStatesResponse
 */
export interface ListDashboardStatesResponse {
  /**
   * List of dashboard states.
   */
  dashboardStates?: DashboardState[];

  /**
   * If response was paged, represents the token for getting the next page of results.
   */
  nextPageToken?: string;

  /**
   * Total count of available dashboard states from the original request.
   */
  totalSize?: number;
}

/**
 * google.android.perf.v1.DeleteDashboardStateRequest
 */
export interface DeleteDashboardStateRequest {
  /**
   * Identifier of the dashboard state to delete.
   */
  name: string;

  /**
   * Concurrent overwriting guard identifier.
   */
  etag?: string;

  /**
   * If true, the API will only check if this request is valid.
   */
  validateOnly?: boolean;

  /**
   * Whether this request supports missing properties.
   */
  allowMissing?: boolean;

  /**
   * Identifier for telemetry.
   */
  requestId?: string;
}

/**
 * google.android.perf.v1.UndeleteDashboardStateRequest
 */
export interface UndeleteDashboardStateRequest {
  /**
   * Identifier of a deleted dashboard state marked for deletion.
   */
  name: string;

  /**
   * Concurrent overwriting guard identifier.
   */
  etag?: string;

  /**
   * Identifier for telemetry.
   */
  requestId?: string;

  /**
   * If true, the API will only check if this request is valid.
   */
  validateOnly?: boolean;
}

/**
 * google.android.perf.v1.ListDashboardStateRevisionsRequest
 */
export interface ListDashboardStateRevisionsRequest {
  /**
   * Identifier for the revisions associated with a given dashboard state.
   */
  name: string;

  /**
   * Amount of revisions to return per request.
   */
  pageSize?: number;

  /**
   * If paginated, represents a token for grabbing the next page of results.
   */
  pageToken?: string;
}

/**
 * google.android.perf.v1.ListDashboardStateRevisionsResponse
 */
export interface ListDashboardStateRevisionsResponse {
  /**
   * List of dashboard state history from the revisions table.
   */
  dashboardStates?: DashboardState[];

  /**
   * Token to grab the next page of results, if applicable.
   */
  nextPageToken?: string;
}

/**
 * google.android.perf.v1.GetDashboardStateRevisionRequest
 */
export interface GetDashboardStateRevisionRequest {
  /**
   * Dashboard state identifier.
   */
  name: string;
}

/**
 * google.android.perf.v1.RollbackDashboardStateRequest
 */
export interface RollbackDashboardStateRequest {
  /**
   * Dashboard state identifier.
   */
  name: string;

  /**
   * Identifier of the dashboard state revision to rollback.
   */
  revisionId: string;

  /**
   * Concurrent overwriting guard identifier.
   */
  etag?: string;

  /**
   * If true, the API will only check if this request is valid.
   */
  validateOnly?: boolean;
}
