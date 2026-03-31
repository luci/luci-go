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

import { Column } from '@/crystal_ball/constants';
import {
  PerfFilter,
  PerfFilterDefault_FilterOperator,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { buildFilterString } from './filter_utils';

describe('buildFilterString', () => {
  const mockGlobalFilters: PerfFilter[] = [
    {
      id: 'global-1',
      column: Column.ATP_TEST_NAME,
      displayName: 'Global 1',
      dataSpecId: 'data-spec-id',
      textInput: {
        defaultValue: {
          values: ['globalValue1'],
          filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
        },
      },
    },
    {
      id: 'global-2',
      column: 'other_column',
      displayName: 'Global 2',
      dataSpecId: 'data-spec-id',
      textInput: {
        defaultValue: {
          values: ['globalValue2'],
          filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
        },
      },
    },
  ];

  const mockWidgetFilters: PerfFilter[] = [
    {
      id: 'widget-1',
      column: Column.ATP_TEST_NAME,
      displayName: 'Widget 1',
      dataSpecId: 'data-spec-id',
      textInput: {
        defaultValue: {
          values: ['widgetValue1'],
          filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
        },
      },
    },
    {
      id: 'widget-2',
      column: 'numeric_column',
      displayName: 'Widget 2',
      dataSpecId: 'data-spec-id',
      numberInput: {
        defaultValue: {
          values: ['123'],
          filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
        },
      },
    },
  ];

  it('filters by a single column', () => {
    const result = buildFilterString(
      [Column.ATP_TEST_NAME],
      mockGlobalFilters,
      mockWidgetFilters,
    );
    expect(result).toBe(
      'atp_test_name = "globalValue1" AND atp_test_name = "widgetValue1"',
    );
  });

  it('filters by multiple columns', () => {
    const result = buildFilterString(
      [Column.ATP_TEST_NAME, 'other_column'],
      mockGlobalFilters,
      mockWidgetFilters,
    );
    expect(result).toBe(
      'atp_test_name = "globalValue1" AND other_column = "globalValue2" AND atp_test_name = "widgetValue1"',
    );
  });

  it('handles numeric filters without quotes', () => {
    const result = buildFilterString(
      ['numeric_column'],
      mockGlobalFilters,
      mockWidgetFilters,
    );
    expect(result).toBe('numeric_column = 123');
  });

  it('excludes current filter', () => {
    const result = buildFilterString(
      [Column.ATP_TEST_NAME],
      mockGlobalFilters,
      mockWidgetFilters,
      'global-1',
    );
    expect(result).toBe('atp_test_name = "widgetValue1"');
  });

  it('returns empty string if no filters match', () => {
    const result = buildFilterString(
      ['non_existent_column'],
      mockGlobalFilters,
      mockWidgetFilters,
    );
    expect(result).toBe('');
  });

  it('handles undefined filters gracefully', () => {
    const result = buildFilterString([Column.ATP_TEST_NAME]);
    expect(result).toBe('');
  });

  it('eliminates duplicates', () => {
    const duplicateFilters: PerfFilter[] = [
      {
        id: 'dup-1',
        column: Column.ATP_TEST_NAME,
        displayName: 'Dup 1',
        dataSpecId: 'data-spec-id',
        textInput: {
          defaultValue: {
            values: ['value1'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
      {
        id: 'dup-2',
        column: Column.ATP_TEST_NAME,
        displayName: 'Dup 2',
        dataSpecId: 'data-spec-id',
        textInput: {
          defaultValue: {
            values: ['value1'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    const result = buildFilterString([Column.ATP_TEST_NAME], duplicateFilters);
    expect(result).toBe('atp_test_name = "value1"');
  });

  it('handles STARTS_WITH operator', () => {
    const filters: PerfFilter[] = [
      {
        id: '1',
        column: Column.ATP_TEST_NAME,
        displayName: 'Test',
        dataSpecId: 'data-spec-id',
        textInput: {
          defaultValue: {
            values: ['v2/android'],
            filterOperator: PerfFilterDefault_FilterOperator.STARTS_WITH,
          },
        },
      },
    ];
    const result = buildFilterString([Column.ATP_TEST_NAME], filters);
    expect(result).toBe('atp_test_name = "v2/android*"');
  });

  it('handles NOT_EQUAL operator', () => {
    const filters: PerfFilter[] = [
      {
        id: '1',
        column: Column.ATP_TEST_NAME,
        displayName: 'Test',
        dataSpecId: 'data-spec-id',
        textInput: {
          defaultValue: {
            values: ['v2/android'],
            filterOperator: PerfFilterDefault_FilterOperator.NOT_EQUAL,
          },
        },
      },
    ];
    const result = buildFilterString([Column.ATP_TEST_NAME], filters);
    expect(result).toBe('atp_test_name != "v2/android"');
  });
});
