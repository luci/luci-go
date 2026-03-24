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

import {
  MeasurementFilterColumn_ColumnDataType,
  PerfFilterDefault_FilterOperator,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * A mapping of PerfFilterDefault_FilterOperator to friendly display names and symbols.
 * Short symbols are used where applicable to save UI space.
 */
export const OPERATOR_DISPLAY_NAMES: Record<
  PerfFilterDefault_FilterOperator,
  string
> = {
  [PerfFilterDefault_FilterOperator.EQUAL]: '=',
  [PerfFilterDefault_FilterOperator.NOT_EQUAL]: '!=',
  [PerfFilterDefault_FilterOperator.IN]: 'in',
  [PerfFilterDefault_FilterOperator.NOT_IN]: 'not in',
  [PerfFilterDefault_FilterOperator.REGEX_MATCH]: 'matches regex',
  [PerfFilterDefault_FilterOperator.NOT_REGEX_MATCH]: 'does not match regex',
  [PerfFilterDefault_FilterOperator.IS_EMPTY]: 'is empty',
  [PerfFilterDefault_FilterOperator.IS_NOT_EMPTY]: 'is not empty',
  [PerfFilterDefault_FilterOperator.STARTS_WITH]: 'starts with',
  [PerfFilterDefault_FilterOperator.NOT_STARTS_WITH]: 'does not start with',
  [PerfFilterDefault_FilterOperator.ENDS_WITH]: 'ends with',
  [PerfFilterDefault_FilterOperator.NOT_ENDS_WITH]: 'does not end with',
  [PerfFilterDefault_FilterOperator.CONTAINS]: 'contains',
  [PerfFilterDefault_FilterOperator.NOT_CONTAINS]: 'does not contain',
  [PerfFilterDefault_FilterOperator.LIKE]: 'like',
  [PerfFilterDefault_FilterOperator.NOT_LIKE]: 'not like',
  [PerfFilterDefault_FilterOperator.GREATER_THAN]: '>',
  [PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL]: '>=',
  [PerfFilterDefault_FilterOperator.LESS_THAN]: '<',
  [PerfFilterDefault_FilterOperator.LESS_THAN_OR_EQUAL]: '<=',
  [PerfFilterDefault_FilterOperator.BETWEEN]: 'between',
  [PerfFilterDefault_FilterOperator.IS_TRUE]: 'is true',
  [PerfFilterDefault_FilterOperator.IS_FALSE]: 'is false',
  [PerfFilterDefault_FilterOperator.IS_NULL]: 'is null',
  [PerfFilterDefault_FilterOperator.IS_NOT_NULL]: 'is not null',
  [PerfFilterDefault_FilterOperator.IN_PAST]: 'in past',
  [PerfFilterDefault_FilterOperator.FILTER_OPERATOR_UNSPECIFIED]: 'unspecified',
};

/**
 * A mapping of MeasurementFilterColumn_ColumnDataType to applicable PerfFilterDefault_FilterOperator enums.
 * This determines which operators are displayed in the UI dropdown for each data type.
 */
export const TYPE_TO_OPERATORS: Record<
  MeasurementFilterColumn_ColumnDataType,
  PerfFilterDefault_FilterOperator[]
> = {
  [MeasurementFilterColumn_ColumnDataType.COLUMN_DATA_TYPE_UNSPECIFIED]: [],
  [MeasurementFilterColumn_ColumnDataType.STRING]: [
    PerfFilterDefault_FilterOperator.EQUAL,
    PerfFilterDefault_FilterOperator.NOT_EQUAL,
    PerfFilterDefault_FilterOperator.IN,
    PerfFilterDefault_FilterOperator.NOT_IN,
    PerfFilterDefault_FilterOperator.REGEX_MATCH,
    PerfFilterDefault_FilterOperator.NOT_REGEX_MATCH,
    PerfFilterDefault_FilterOperator.IS_EMPTY,
    PerfFilterDefault_FilterOperator.IS_NOT_EMPTY,
    PerfFilterDefault_FilterOperator.STARTS_WITH,
    PerfFilterDefault_FilterOperator.NOT_STARTS_WITH,
    PerfFilterDefault_FilterOperator.ENDS_WITH,
    PerfFilterDefault_FilterOperator.NOT_ENDS_WITH,
    PerfFilterDefault_FilterOperator.CONTAINS,
    PerfFilterDefault_FilterOperator.NOT_CONTAINS,
    PerfFilterDefault_FilterOperator.LIKE,
    PerfFilterDefault_FilterOperator.NOT_LIKE,
  ],
  [MeasurementFilterColumn_ColumnDataType.INT64]: [
    PerfFilterDefault_FilterOperator.EQUAL,
    PerfFilterDefault_FilterOperator.NOT_EQUAL,
    PerfFilterDefault_FilterOperator.GREATER_THAN,
    PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL,
    PerfFilterDefault_FilterOperator.LESS_THAN,
    PerfFilterDefault_FilterOperator.LESS_THAN_OR_EQUAL,
    PerfFilterDefault_FilterOperator.BETWEEN,
    PerfFilterDefault_FilterOperator.IN,
    PerfFilterDefault_FilterOperator.NOT_IN,
  ],
  [MeasurementFilterColumn_ColumnDataType.DOUBLE]: [
    PerfFilterDefault_FilterOperator.EQUAL,
    PerfFilterDefault_FilterOperator.NOT_EQUAL,
    PerfFilterDefault_FilterOperator.GREATER_THAN,
    PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL,
    PerfFilterDefault_FilterOperator.LESS_THAN,
    PerfFilterDefault_FilterOperator.LESS_THAN_OR_EQUAL,
    PerfFilterDefault_FilterOperator.BETWEEN,
    PerfFilterDefault_FilterOperator.IN,
    PerfFilterDefault_FilterOperator.NOT_IN,
  ],
  [MeasurementFilterColumn_ColumnDataType.BOOLEAN]: [
    PerfFilterDefault_FilterOperator.IS_TRUE,
    PerfFilterDefault_FilterOperator.IS_FALSE,
  ],
  [MeasurementFilterColumn_ColumnDataType.TIMESTAMP]: [
    PerfFilterDefault_FilterOperator.EQUAL,
    PerfFilterDefault_FilterOperator.NOT_EQUAL,
    PerfFilterDefault_FilterOperator.GREATER_THAN,
    PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL,
    PerfFilterDefault_FilterOperator.LESS_THAN,
    PerfFilterDefault_FilterOperator.LESS_THAN_OR_EQUAL,
    PerfFilterDefault_FilterOperator.BETWEEN,
    PerfFilterDefault_FilterOperator.IN_PAST,
  ],
  [MeasurementFilterColumn_ColumnDataType.DATE]: [
    PerfFilterDefault_FilterOperator.EQUAL,
    PerfFilterDefault_FilterOperator.NOT_EQUAL,
    PerfFilterDefault_FilterOperator.GREATER_THAN,
    PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL,
    PerfFilterDefault_FilterOperator.LESS_THAN,
    PerfFilterDefault_FilterOperator.LESS_THAN_OR_EQUAL,
    PerfFilterDefault_FilterOperator.BETWEEN,
  ],
};
