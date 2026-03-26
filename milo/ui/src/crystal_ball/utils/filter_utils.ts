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
  PerfFilter,
  PerfFilterDefault_FilterOperator,
  perfFilterDefault_FilterOperatorFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

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
  columnsToFilterBy: string[],
  globalFilters?: readonly PerfFilter[],
  widgetFilters?: readonly PerfFilter[],
  currentFilterId?: string,
): string => {
  const parsedFilters: {
    column: string;
    value: string;
    operator: PerfFilterDefault_FilterOperator;
    type: 'number' | 'string';
  }[] = [];

  const addFilters = (filtersToProcess?: readonly PerfFilter[]) => {
    if (!filtersToProcess) return;
    filtersToProcess.forEach((f) => {
      if (columnsToFilterBy.includes(f.column) && f.id !== currentFilterId) {
        const val =
          f.textInput?.defaultValue?.values?.[0] ??
          f.numberInput?.defaultValue?.values?.[0];
        const isNumber = Boolean(f.numberInput);
        if (val) {
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
          parsedFilters.push({
            column: f.column,
            value: val,
            operator: op,
            type: isNumber ? 'number' : 'string',
          });
        }
      }
    });
  };

  addFilters(globalFilters);
  addFilters(widgetFilters);

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
      let val = f.value;
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
        switch (f.operator) {
          case PerfFilterDefault_FilterOperator.STARTS_WITH:
            if (!val.includes('*') && !val.includes('%')) {
              val += '*';
            }
            opStr = '=';
            break;
          case PerfFilterDefault_FilterOperator.NOT_EQUAL:
            opStr = '!=';
            break;
          case PerfFilterDefault_FilterOperator.EQUAL:
          default:
            opStr = '=';
            break;
        }
        const formattedVal = val.replace(/"/g, '\\"');
        return `${f.column} ${opStr} "${formattedVal}"`;
      }
    })
    .join(' AND ');
};
