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

import { DateTime } from 'luxon';

import {
  PerfFilter,
  PerfFilterDefault_FilterOperator,
  perfFilterDefault_FilterOperatorFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Internal representation of a parsed filter.
 */
export interface ParsedFilter {
  column: string;
  value: string | readonly string[];
  operator: PerfFilterDefault_FilterOperator;
  type: 'number' | 'string';
}

/**
 * Parses a single PerfFilter into an array of internal ParsedFilter objects.
 * Handles range filters (IN_PAST, BETWEEN) and simple text/number filters.
 * Returns empty array if the filter should be excluded (wrong column or current filter ID).
 */
export const parseSingleFilter = (
  f: PerfFilter,
  currentFilterId?: string,
): ParsedFilter[] => {
  if (f.id === currentFilterId) {
    return [];
  }

  const results: ParsedFilter[] = [];

  if (f.range) {
    const rangeOp =
      f.range.defaultValue?.filterOperator !== undefined
        ? perfFilterDefault_FilterOperatorFromJSON(
            f.range.defaultValue.filterOperator,
          )
        : PerfFilterDefault_FilterOperator.IN_PAST;
    const values = f.range.defaultValue?.values ?? [];

    if (rangeOp === PerfFilterDefault_FilterOperator.IN_PAST && values[0]) {
      const match = values[0].match(/^(\d+)([a-zA-Z])$/);
      if (match) {
        const amount = parseInt(match[1], 10);
        const unit = match[2];
        const luxonUnitMap: Record<string, string> = {
          m: 'minutes',
          h: 'hours',
          d: 'days',
          w: 'weeks',
        };
        const luxonUnit = luxonUnitMap[unit] ?? 'days';
        const computedDate = DateTime.now()
          .minus({ [luxonUnit]: amount })
          .toUTC()
          .toISO();
        if (computedDate) {
          results.push({
            column: f.column,
            value: computedDate,
            operator: PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL,
            type: 'string',
          });
        }
      }
    } else if (
      rangeOp === PerfFilterDefault_FilterOperator.BETWEEN &&
      values.length >= 2
    ) {
      if (values[0]) {
        results.push({
          column: f.column,
          value: values[0],
          operator: PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL,
          type: 'string',
        });
      }
      if (values[1]) {
        results.push({
          column: f.column,
          value: values[1],
          operator: PerfFilterDefault_FilterOperator.LESS_THAN_OR_EQUAL,
          type: 'string',
        });
      }
    }
  } else {
    const values =
      f.textInput?.defaultValue?.values ??
      f.numberInput?.defaultValue?.values ??
      [];
    const isNumber = Boolean(f.numberInput);
    const op =
      f.textInput?.defaultValue?.filterOperator !== undefined
        ? perfFilterDefault_FilterOperatorFromJSON(
            f.textInput.defaultValue.filterOperator,
          )
        : f.numberInput?.defaultValue?.filterOperator !== undefined
          ? perfFilterDefault_FilterOperatorFromJSON(
              f.numberInput.defaultValue.filterOperator,
            )
          : PerfFilterDefault_FilterOperator.EQUAL;

    if (
      op === PerfFilterDefault_FilterOperator.IN ||
      op === PerfFilterDefault_FilterOperator.NOT_IN
    ) {
      if (values.length > 0) {
        results.push({
          column: f.column,
          value: values,
          operator: op,
          type: isNumber ? 'number' : 'string',
        });
      }
    } else {
      const val = values[0];
      if (val) {
        results.push({
          column: f.column,
          value: val,
          operator: op,
          type: isNumber ? 'number' : 'string',
        });
      }
    }
  }

  return results;
};

/**
 * Builds an AIP-160 compliant filter string for specified columns based on
 * provided global and widget-level filters.
 *
 * @param columnsToFilterBy - Array of column names to include in the filter.
 * @param globalFilters - Optional readonly array of global filters.
 * @param widgetFilters - Optional readonly array of widget-level filters.
 * @param currentFilterId - Optional ID of the filter currently being edited, to exclude from the result.
 * @returns An AIP-160 compliant filter string.
 */
export const buildFilterString = (
  filters: readonly PerfFilter[],
  currentFilterId?: string,
): string => {
  const parsedFilters: ParsedFilter[] = [];

  filters.forEach((f) => {
    parsedFilters.push(...parseSingleFilter(f, currentFilterId));
  });

  // Eliminate duplicates
  const uniqueFilters = parsedFilters.filter(
    (f, index, self) =>
      index ===
      self.findIndex(
        (t) =>
          t.column === f.column &&
          t.value === f.value &&
          t.operator === f.operator,
      ),
  );

  return uniqueFilters
    .map((f) => {
      const vals = Array.isArray(f.value) ? f.value : [f.value];

      if (
        f.operator === PerfFilterDefault_FilterOperator.IN ||
        f.operator === PerfFilterDefault_FilterOperator.NOT_IN
      ) {
        const opStr =
          f.operator === PerfFilterDefault_FilterOperator.IN ? 'IN' : 'NOT IN';
        const formattedVals = vals
          .map((v) => (f.type === 'number' ? v : `"${v.replace(/"/g, '\\"')}"`))
          .join(', ');
        return `${f.column} ${opStr} (${formattedVals})`;
      }

      const val = vals[0] ?? '';
      let opStr = '=';

      if (f.type === 'number') {
        switch (f.operator) {
          case PerfFilterDefault_FilterOperator.NOT_EQUAL:
            opStr = '!=';
            break;
          case PerfFilterDefault_FilterOperator.GREATER_THAN:
            opStr = '>';
            break;
          case PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL:
            opStr = '>=';
            break;
          case PerfFilterDefault_FilterOperator.LESS_THAN:
            opStr = '<';
            break;
          case PerfFilterDefault_FilterOperator.LESS_THAN_OR_EQUAL:
            opStr = '<=';
            break;
          case PerfFilterDefault_FilterOperator.EQUAL:
          default:
            opStr = '=';
            break;
        }
        return `${f.column} ${opStr} ${val}`;
      } else {
        let finalVal = val;
        switch (f.operator) {
          case PerfFilterDefault_FilterOperator.STARTS_WITH:
            if (!finalVal.includes('*') && !finalVal.includes('%')) {
              finalVal += '*';
            }
            opStr = '=';
            break;
          case PerfFilterDefault_FilterOperator.NOT_EQUAL:
            opStr = '!=';
            break;
          case PerfFilterDefault_FilterOperator.GREATER_THAN:
            opStr = '>';
            break;
          case PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL:
            opStr = '>=';
            break;
          case PerfFilterDefault_FilterOperator.LESS_THAN:
            opStr = '<';
            break;
          case PerfFilterDefault_FilterOperator.LESS_THAN_OR_EQUAL:
            opStr = '<=';
            break;
          case PerfFilterDefault_FilterOperator.EQUAL:
          default:
            opStr = '=';
            break;
        }
        const formattedVal = finalVal.replace(/"/g, '\\"');
        return `${f.column} ${opStr} "${formattedVal}"`;
      }
    })
    .join(' AND ');
};

/**
 * Generates a human-readable label for a PerfFilter, handling multiple values for IN and NOT_IN operators.
 */
export const getFilterLabel = (
  filter: PerfFilter,
  operatorDisplayNames: Record<PerfFilterDefault_FilterOperator, string>,
): string => {
  const values =
    filter.textInput?.defaultValue?.values ??
    filter.numberInput?.defaultValue?.values ??
    [];
  const op =
    filter.textInput?.defaultValue?.filterOperator !== undefined
      ? perfFilterDefault_FilterOperatorFromJSON(
          filter.textInput.defaultValue.filterOperator,
        )
      : filter.numberInput?.defaultValue?.filterOperator !== undefined
        ? perfFilterDefault_FilterOperatorFromJSON(
            filter.numberInput.defaultValue.filterOperator,
          )
        : PerfFilterDefault_FilterOperator.EQUAL;

  const opStr =
    operatorDisplayNames[op] ?? PerfFilterDefault_FilterOperator[op];
  const isNumber = Boolean(filter.numberInput);

  if (
    op === PerfFilterDefault_FilterOperator.IN ||
    op === PerfFilterDefault_FilterOperator.NOT_IN
  ) {
    const formattedVals = values
      .map((v) => (isNumber ? v : `"${v}"`))
      .join(', ');
    return `${filter.column} ${opStr} (${formattedVals})`;
  }

  const val = values[0] ?? '';
  return `${filter.column} ${opStr} ${isNumber ? val : `"${val}"`}`;
};
