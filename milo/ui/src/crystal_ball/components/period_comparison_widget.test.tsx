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

import { fireEvent, render as rtlRender, screen } from '@testing-library/react';
import { DateTime } from 'luxon';
import { useState } from 'react';

import { Column } from '@/crystal_ball/constants';
import { FiltersClipboardProvider } from '@/crystal_ball/context';
import {
  UseEditorUiStateOptions,
  useFetchDashboardWidgetData,
} from '@/crystal_ball/hooks';
import { createMockQueryResult } from '@/crystal_ball/tests';
import {
  FetchDashboardWidgetDataResponse,
  PerfChartSeries_PerfAggregationFunction,
  PerfChartWidget,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { PeriodComparisonWidget } from './period_comparison_widget';

jest.mock('@/crystal_ball/hooks', () => ({
  ...jest.requireActual('@/crystal_ball/hooks'),
  useEditorUiState: ({ initialValue = false }: UseEditorUiStateOptions) => {
    const [val, setVal] = useState(initialValue);
    return [val, setVal];
  },
  useFetchDashboardWidgetData: jest.fn(),
  useToast: () => ({
    showSuccessToast: jest.fn(),
    showWarningToast: jest.fn(),
    showErrorToast: jest.fn(),
  }),
}));

jest.mock('@/crystal_ball/hooks/use_measurement_filter_api', () => ({
  useListMeasurementFilterColumns: jest.fn(() => ({
    data: { columns: [] },
    isLoading: false,
  })),
  useSuggestMeasurementFilterValues: jest.fn(() => ({
    data: { values: [] },
    isLoading: false,
  })),
}));

jest.mock('@mui/x-date-pickers/DateTimePicker', () => ({
  DateTimePicker: ({
    label,
    value,
    onChange,
  }: {
    label: string;
    value: DateTime | null;
    onChange: (date: DateTime | null) => void;
  }) => (
    <div>
      <label>{label}</label>
      <input
        type="text"
        data-testid={`date-picker-${label.toLowerCase().replace(/\s/g, '-')}`}
        value={value ? value.toISO() : ''}
        onChange={(e) =>
          onChange(e.target.value ? DateTime.fromISO(e.target.value) : null)
        }
      />
    </div>
  ),
}));

const mockUseFetchDashboardWidgetData = jest.mocked(
  useFetchDashboardWidgetData,
);

const baseWidget = PerfChartWidget.fromPartial({
  displayName: 'Test Period Comparison',
  series: [
    {
      metricField: 'test_metric',
      filters: [
        {
          column: Column.ATP_TEST_NAME,
          textInput: { defaultValue: { values: ['test_value'] } },
        },
      ],
    },
  ],
  periodComparisonChartConfig: {
    aggregations: [PerfChartSeries_PerfAggregationFunction.MEAN],
    baselineTimeRange: {
      startTime: '2026-05-01T00:00:00Z',
      endTime: '2026-05-02T00:00:00Z',
    },
    comparisonTimeRange: {
      startTime: '2026-05-03T00:00:00Z',
      endTime: '2026-05-04T00:00:00Z',
    },
  },
});

const AllProviders = ({ children }: { children: React.ReactNode }) => (
  <FiltersClipboardProvider>{children}</FiltersClipboardProvider>
);

const render = (ui: React.ReactElement) =>
  rtlRender(ui, { wrapper: AllProviders });

describe('PeriodComparisonWidget', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('calls useFetchDashboardWidgetData with correct arguments', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          periodComparisonData: { series: [] },
        }),
      ),
    );

    render(
      <PeriodComparisonWidget
        widget={baseWidget}
        dashboardName="dashboardStates/test-dashboard"
        widgetId="w1"
        onUpdate={jest.fn()}
        filterColumns={[]}
      />,
    );

    expect(mockUseFetchDashboardWidgetData).toHaveBeenCalledWith(
      expect.objectContaining({
        widgetId: 'w1',
      }),
      expect.any(Object),
    );
  });

  it('renders config panel fields and inputs correctly', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          periodComparisonData: { series: [] },
        }),
      ),
    );

    render(
      <PeriodComparisonWidget
        widget={baseWidget}
        dashboardName="dashboardStates/test-dashboard"
        widgetId="w1"
        onUpdate={jest.fn()}
        filterColumns={[]}
      />,
    );

    expect(screen.getByText(/Baseline Period/i)).toBeInTheDocument();
    expect(screen.getByText(/Comparison Period/i)).toBeInTheDocument();
    expect(screen.getByText(/test_metric/i)).toBeInTheDocument();
  });

  it('renders comparative stats details in table on success', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          periodComparisonData: {
            series: [
              {
                seriesId: 's1',
                legendLabel: 'Legend Label',
                metricField: 'test_metric',
                aggregationData: [
                  {
                    aggregationType: 1, // MEAN
                    baselineStats: { value: 10.0 },
                    comparisonStats: { value: 12.5 },
                    netChange: 2.5,
                    volatility: 0.5,
                    standardDeviation: 0.2,
                    cumulativeImpact: 1.0,
                    percentageChange: 0.25,
                  },
                ],
              },
            ],
          },
        }),
      ),
    );

    render(
      <PeriodComparisonWidget
        widget={baseWidget}
        dashboardName="dashboardStates/test-dashboard"
        widgetId="w1"
        onUpdate={jest.fn()}
        filterColumns={[]}
      />,
    );

    expect(screen.getByText('Legend Label')).toBeInTheDocument();
    expect(screen.getByText('10.00')).toBeInTheDocument();
    expect(screen.getByText('12.50')).toBeInTheDocument();
    expect(screen.getByText('+2.50')).toBeInTheDocument();
    expect(screen.getByText('+25.00%')).toBeInTheDocument();
  });

  it('calls onUpdate when baseline time range changes', () => {
    const mockOnUpdate = jest.fn();
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          periodComparisonData: { series: [] },
        }),
      ),
    );

    render(
      <PeriodComparisonWidget
        widget={baseWidget}
        dashboardName="dashboardStates/test-dashboard"
        widgetId="w1"
        onUpdate={mockOnUpdate}
        filterColumns={[]}
      />,
    );

    const fromInput = screen.getAllByRole('textbox')[0]; // First "From" input
    fireEvent.change(fromInput, { target: { value: '2026-05-10T00:00:00Z' } });

    expect(mockOnUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        periodComparisonChartConfig: expect.objectContaining({
          baselineTimeRange: expect.objectContaining({
            startTime: '2026-05-10T00:00:00.000+00:00',
          }),
        }),
      }),
    );
  });
});
