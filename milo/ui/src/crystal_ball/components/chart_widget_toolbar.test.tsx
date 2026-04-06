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

import { render, screen, fireEvent } from '@testing-library/react';

import {
  AGGREGATION_FUNCTION_LABELS,
  GROUP_BY_OPTIONS,
} from '@/crystal_ball/constants';
import {
  PerfChartWidget_ChartType,
  PerfChartSeries_PerfAggregationFunction,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { ChartWidgetToolbar } from './chart_widget_toolbar';

describe('ChartWidgetToolbar', () => {
  const mockProps = {
    chartType: PerfChartWidget_ChartType.MULTI_METRIC_CHART,
    onChartTypeChange: jest.fn(),
    currentGroupBy: GROUP_BY_OPTIONS[0].value,
    onGroupByChange: jest.fn(),
    currentAggregation: PerfChartSeries_PerfAggregationFunction.P50,
    onAggregationChange: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('should render correctly with default props', () => {
    render(<ChartWidgetToolbar {...mockProps} />);

    expect(screen.getByTitle('Line Chart')).toBeInTheDocument();
    expect(screen.getByTitle('Scatter Plot')).toBeInTheDocument();
    expect(screen.getByLabelText('Widget Group By')).toBeInTheDocument();
    expect(screen.getByLabelText('Widget Aggregation')).toBeInTheDocument();
  });

  it('should call onChartTypeChange when toggle button is clicked', () => {
    render(<ChartWidgetToolbar {...mockProps} />);

    const scatterPlotButton = screen.getByTitle('Scatter Plot');
    fireEvent.click(scatterPlotButton);

    expect(mockProps.onChartTypeChange).toHaveBeenCalledTimes(1);
    expect(mockProps.onChartTypeChange).toHaveBeenCalledWith(
      PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION,
    );
  });

  it('should call onGroupByChange when group by option is selected', () => {
    render(<ChartWidgetToolbar {...mockProps} />);

    const select = screen.getByLabelText('Widget Group By');
    // MUI Select requires mouseDown to open the menu in tests
    fireEvent.mouseDown(select);

    // Find option by text and click it
    // GROUP_BY_OPTIONS[1] should be available
    const option = screen.getByText(GROUP_BY_OPTIONS[1].label);
    fireEvent.click(option);

    expect(mockProps.onGroupByChange).toHaveBeenCalledTimes(1);
    expect(mockProps.onGroupByChange).toHaveBeenCalledWith(
      GROUP_BY_OPTIONS[1].value,
    );
  });

  it('should call onAggregationChange when aggregation function is selected', () => {
    render(<ChartWidgetToolbar {...mockProps} />);

    const select = screen.getByLabelText('Widget Aggregation');
    fireEvent.mouseDown(select);

    const meanLabel =
      AGGREGATION_FUNCTION_LABELS[
        PerfChartSeries_PerfAggregationFunction.MEAN
      ] ?? 'Mean';
    const option = screen.getByRole('option', { name: meanLabel });
    fireEvent.click(option);

    expect(mockProps.onAggregationChange).toHaveBeenCalledTimes(1);
    expect(mockProps.onAggregationChange).toHaveBeenCalledWith(
      PerfChartSeries_PerfAggregationFunction.MEAN,
    );
  });

  it('should hide aggregation controls when chart type is INVOCATION_DISTRIBUTION', () => {
    render(
      <ChartWidgetToolbar
        {...mockProps}
        chartType={PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION}
      />,
    );

    expect(screen.getByTitle('Line Chart')).toBeInTheDocument();
    expect(screen.getByTitle('Scatter Plot')).toBeInTheDocument();
    expect(screen.getByLabelText('Widget Group By')).toBeInTheDocument();
    expect(
      screen.queryByLabelText('Widget Aggregation'),
    ).not.toBeInTheDocument();
  });
});
