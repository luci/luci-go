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

  it('processes filters correctly', () => {
    const atpFilters = [mockGlobalFilters[0], mockWidgetFilters[0]];
    const result = buildFilterString(atpFilters);
    expect(result).toBe(
      'atp_test_name = "globalValue1" AND atp_test_name = "widgetValue1"',
    );
  });

  it('includes all filters provided', () => {
    const result = buildFilterString([
      ...mockGlobalFilters,
      ...mockWidgetFilters,
    ]);
    expect(result).toBe(
      'atp_test_name = "globalValue1" AND other_column = "globalValue2" AND atp_test_name = "widgetValue1" AND numeric_column = 123',
    );
  });

  it('handles numeric filters without quotes', () => {
    const result = buildFilterString([mockWidgetFilters[1]]);
    expect(result).toBe('numeric_column = 123');
  });

  it('excludes current filter', () => {
    const atpFilters = [mockGlobalFilters[0], mockWidgetFilters[0]];
    const result = buildFilterString(atpFilters, 'global-1');
    expect(result).toBe('atp_test_name = "widgetValue1"');
  });

  it('returns empty string if no filters provided', () => {
    const result = buildFilterString([]);
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
    const result = buildFilterString(duplicateFilters);
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
    const result = buildFilterString(filters);
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
    const result = buildFilterString(filters);
    expect(result).toBe('atp_test_name != "v2/android"');
  });

  it('handles IN_PAST range filters', () => {
    const filters: PerfFilter[] = [
      {
        id: '1',
        column: 'build_creation_timestamp',
        displayName: 'Time Range',
        dataSpecId: 'data-spec-id',
        range: {
          defaultValue: {
            values: ['3d'],
            filterOperator: PerfFilterDefault_FilterOperator.IN_PAST,
          },
        },
      },
    ];
    const result = buildFilterString(filters);
    expect(result).toMatch(
      /^build_creation_timestamp >= "\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/,
    );
  });

  it('handles BETWEEN range filters', () => {
    const filters: PerfFilter[] = [
      {
        id: '1',
        column: 'build_creation_timestamp',
        displayName: 'Time Range',
        dataSpecId: 'data-spec-id',
        range: {
          defaultValue: {
            values: ['2026-03-01T00:00:00Z', '2026-03-04T00:00:00Z'],
            filterOperator: PerfFilterDefault_FilterOperator.BETWEEN,
          },
        },
      },
    ];
    const result = buildFilterString(filters);
    expect(result).toBe(
      'build_creation_timestamp >= "2026-03-01T00:00:00Z" AND build_creation_timestamp <= "2026-03-04T00:00:00Z"',
    );
  });

  it('handles IN operator with multiple values', () => {
    const filters: PerfFilter[] = [
      {
        id: '1',
        column: Column.BUILD_TYPE,
        displayName: 'Build Type',
        dataSpecId: 'data-spec-id',
        textInput: {
          defaultValue: {
            values: ['Postsubmit', 'Presubmit'],
            filterOperator: PerfFilterDefault_FilterOperator.IN,
          },
        },
      },
    ];
    const result = buildFilterString(filters);
    expect(result).toBe('build_type IN ("Postsubmit", "Presubmit")');
  });
});
