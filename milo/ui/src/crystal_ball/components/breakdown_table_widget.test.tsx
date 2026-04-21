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
import { useState } from 'react';

import { Column, COMMON_MESSAGES } from '@/crystal_ball/constants';
import { FiltersClipboardProvider } from '@/crystal_ball/context';
import {
  UseEditorUiStateOptions,
  useFetchDashboardWidgetData,
} from '@/crystal_ball/hooks';
import { createMockQueryResult } from '@/crystal_ball/tests';
import {
  FetchDashboardWidgetDataResponse,
  PerfChartWidget,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { BreakdownTableWidget } from './breakdown_table_widget';

jest.mock('@/crystal_ball/hooks', () => ({
  ...jest.requireActual('@/crystal_ball/hooks'),
  useEditorUiState: ({ initialValue = false }: UseEditorUiStateOptions) => {
    const [val, setVal] = useState(initialValue);
    return [val, setVal];
  },
  useFetchDashboardWidgetData: jest.fn(),
  useSuggestMeasurementFilterValues: jest.fn(() => ({ data: [] })),
  useToast: () => ({
    showSuccessToast: jest.fn(),
    showWarningToast: jest.fn(),
    showErrorToast: jest.fn(),
  }),
}));

const mockUseFetchDashboardWidgetData = jest.mocked(
  useFetchDashboardWidgetData,
);

const baseWidget = PerfChartWidget.fromPartial({
  displayName: 'Test Breakdown',
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
});

const AllProviders = ({ children }: { children: React.ReactNode }) => (
  <FiltersClipboardProvider>{children}</FiltersClipboardProvider>
);

// Override render to automatically include FiltersClipboardProvider,
// avoiding the need to wrap every test case manually and ensuring
// that rerender() also uses the provider automatically.
const render = (ui: React.ReactElement) =>
  rtlRender(ui, { wrapper: AllProviders });

describe('BreakdownTableWidget', () => {
  it('calls useFetchDashboardWidgetData with correct arguments', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          breakdownTableData: { sections: [] },
        }),
      ),
    );

    render(
      <BreakdownTableWidget
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

  it('renders ChartSeriesEditor and BreakdownTableChart', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          breakdownTableData: { sections: [] },
        }),
      ),
    );

    render(
      <BreakdownTableWidget
        widget={baseWidget}
        dashboardName="dashboardStates/test-dashboard"
        widgetId="w1"
        onUpdate={jest.fn()}
        filterColumns={[]}
      />,
    );

    // ChartSeriesItem should be visible
    expect(screen.getByText(/test_metric/)).toBeInTheDocument();

    // BreakdownTableChart should be visible (it will render the "No Data" message if sections are empty)
    // We can check for the "No data found" message which is rendered by the chart.
    expect(screen.getByText(COMMON_MESSAGES.NO_DATA_FOUND)).toBeInTheDocument();
  });

  it('calls onUpdate when dimension selection changes', () => {
    const mockOnUpdate = jest.fn();
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          breakdownTableData: {
            sections: [
              { dimensionColumn: 'testname', rows: [] },
              { dimensionColumn: 'buildbranch', rows: [] },
            ],
          },
        }),
      ),
    );

    render(
      <BreakdownTableWidget
        widget={baseWidget}
        dashboardName="dashboardStates/test-dashboard"
        widgetId="w1"
        onUpdate={mockOnUpdate}
        filterColumns={[]}
      />,
    );

    const select = screen.getByRole('combobox', {
      name: 'Breakdown by category',
    });
    fireEvent.mouseDown(select);

    const option = screen.getByRole('option', { name: /BUILDBRANCH/i });
    fireEvent.click(option);

    expect(mockOnUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        breakdownTableWidgetChartConfig: expect.objectContaining({
          defaultDimension: 'buildbranch',
        }),
      }),
    );
  });
});
