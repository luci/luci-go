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
  MeasurementFilterColumn,
  MeasurementFilterColumn_FilterScope,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import {
  buildFilterString,
  formatColumnNameFallback,
  getColumnDisplayNameMap,
  getDimensionLabel,
  getFilterableColumns,
} from './filter_utils';

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

describe('formatColumnNameFallback and getColumnDisplayNameMap', () => {
  it('strips dim. prefix in formatColumnNameFallback', () => {
    expect(formatColumnNameFallback('dim.device_id')).toBe('DEVICE ID');
    expect(formatColumnNameFallback('build_target')).toBe('BUILD TARGET');
  });

  it('maps both dim. and non-dim. column names in getColumnDisplayNameMap', () => {
    const map = getColumnDisplayNameMap([
      MeasurementFilterColumn.fromPartial({
        column: 'device_id',
        displayName: 'Device Identifier',
      }),
      MeasurementFilterColumn.fromPartial({
        column: 'dim.gpu_vendor',
        displayName: 'GPU Vendor',
      }),
    ]);

    expect(map['device_id']).toBe('Device Identifier');
    expect(map['dim.device_id']).toBe('Device Identifier');
    expect(map['gpu_vendor']).toBe('GPU Vendor');
    expect(map['dim.gpu_vendor']).toBe('GPU Vendor');
  });

  it('resolves dimension labels with getDimensionLabel respecting precedence', () => {
    const map = {
      'dim.device_id': 'Device Identifier',
      gpu_vendor: 'GPU Vendor',
    };

    // 1. Explicit display name takes highest precedence
    expect(getDimensionLabel('dim.device_id', 'Custom Name', map)).toBe(
      'Custom Name',
    );

    // 2. Direct map lookup
    expect(getDimensionLabel('dim.device_id', undefined, map)).toBe(
      'Device Identifier',
    );

    // 3. Stripped dim. prefix lookup
    expect(getDimensionLabel('dim.gpu_vendor', undefined, map)).toBe(
      'GPU Vendor',
    );

    // 4. Fallback formatting
    expect(getDimensionLabel('dim.unknown_col', undefined, map)).toBe(
      'UNKNOWN COL',
    );
    expect(getDimensionLabel('build_branch', undefined, undefined)).toBe(
      'BUILD BRANCH',
    );
  });
});

describe('getFilterableColumns', () => {
  it('excludes statistical_key, isMetricKey, and global time range columns', () => {
    const columns = [
      MeasurementFilterColumn.fromPartial({
        column: 'build_target',
        displayName: 'Build Target',
        applicableScopes: [MeasurementFilterColumn_FilterScope.WIDGET],
      }),
      MeasurementFilterColumn.fromPartial({
        column: 'statistical_key',
        displayName: 'Statistical Key',
        applicableScopes: [MeasurementFilterColumn_FilterScope.WIDGET],
      }),
      MeasurementFilterColumn.fromPartial({
        column: 'build_creation_timestamp',
        displayName: 'Time Range',
        applicableScopes: [MeasurementFilterColumn_FilterScope.WIDGET],
      }),
      MeasurementFilterColumn.fromPartial({
        column: 'metric_key',
        displayName: 'Metric Key',
        isMetricKey: true,
        applicableScopes: [MeasurementFilterColumn_FilterScope.WIDGET],
      }),
      MeasurementFilterColumn.fromPartial({
        column: 'dim.device_id',
        displayName: 'Device Identifier',
        applicableScopes: [MeasurementFilterColumn_FilterScope.WIDGET],
      }),
    ];

    const result = getFilterableColumns(columns);
    expect(result.map((c) => c.column)).toEqual([
      'build_target',
      'dim.device_id',
    ]);
  });
});
