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

import '@testing-library/jest-dom';
import {
  SelectedPointInfo,
  SampleDetailsContent,
} from '@/crystal_ball/components';
import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { useFetchWidgetRawSamples } from '@/crystal_ball/hooks';
import {
  createMockQueryResult,
  createMockPendingResult,
  createMockErrorResult,
} from '@/crystal_ball/tests';
import {
  PerfChartSeries_PerfAggregationFunction,
  PerfChartWidget,
  PerfChartWidget_ChartType,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

// Mock the hook
jest.mock('@/crystal_ball/hooks', () => ({
  ...jest.requireActual('@/crystal_ball/hooks'),
  useFetchWidgetRawSamples: jest.fn(),
}));

const mockUseFetchWidgetRawSamples = jest.mocked(useFetchWidgetRawSamples);

describe('SampleDetailsContent', () => {
  const mockPoint: SelectedPointInfo = {
    componentType: 'series',
    seriesName: 'mock_series',
    x: 1000,
    y: 50,
    count: 1,
    point: { test_name: 'test#method' },
    seriesId: 's1',
    seriesIndex: 0,
  };

  const mockWidget: PerfChartWidget = PerfChartWidget.fromPartial({
    chartType: PerfChartWidget_ChartType.MULTI_METRIC_CHART,
    series: [
      {
        id: 's1',
        color: '#ff0000',
        aggregation: PerfChartSeries_PerfAggregationFunction.MEAN,
      },
    ],
  });

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('should display loading state', () => {
    mockUseFetchWidgetRawSamples.mockReturnValue(createMockPendingResult());
    render(
      <SampleDetailsContent
        selectedPoint={mockPoint}
        widgetId="w1"
        widget={mockWidget}
        dashboardName="dashboard1"
      />,
    );
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  it('should display error state', () => {
    mockUseFetchWidgetRawSamples.mockReturnValue(
      createMockErrorResult(new Error('Failed to fetch')),
    );
    render(
      <SampleDetailsContent
        selectedPoint={mockPoint}
        widgetId="w1"
        widget={mockWidget}
        dashboardName="dashboard1"
      />,
    );
    expect(screen.getByText('Failed to fetch')).toBeInTheDocument();
  });

  it('should display empty state', () => {
    mockUseFetchWidgetRawSamples.mockReturnValue(
      createMockQueryResult({ rows: [], nextPageToken: '' }),
    );
    render(
      <SampleDetailsContent
        selectedPoint={mockPoint}
        widgetId="w1"
        widget={mockWidget}
        dashboardName="dashboard1"
      />,
    );
    expect(
      screen.getByText(COMMON_MESSAGES.NO_RAW_SAMPLES_FOUND),
    ).toBeInTheDocument();
  });

  it('should render rows with data', () => {
    mockUseFetchWidgetRawSamples.mockReturnValue(
      createMockQueryResult({
        nextPageToken: '',
        rows: [
          {
            values: {
              value: '100',
              test_name: 'test_class#test_method',
              atp_test_name: 'atp_test',
              build_id: '123',
              build_branch: 'main',
              build_target: 'target',
              invocation_complete_timestamp: '2026-04-04T10:00:00Z',
            },
          },
        ],
      }),
    );

    render(
      <SampleDetailsContent
        selectedPoint={mockPoint}
        widgetId="w1"
        widget={mockWidget}
        dashboardName="dashboard1"
      />,
    );

    expect(screen.getAllByText('100')).not.toHaveLength(0);
    expect(screen.getAllByText('atp_test')).not.toHaveLength(0);
    expect(screen.getAllByText(/test_class/)).not.toHaveLength(0);
    expect(screen.getAllByText(/#test_method/)).not.toHaveLength(0);
    expect(screen.getAllByText('123')).not.toHaveLength(0);
    expect(screen.getAllByText('main')).not.toHaveLength(0);
    expect(screen.getAllByText('target')).not.toHaveLength(0);
  });

  it('should display descriptive sentence in header (plural)', () => {
    mockUseFetchWidgetRawSamples.mockReturnValue(
      createMockQueryResult({ rows: [], nextPageToken: '' }),
    );
    render(
      <SampleDetailsContent
        selectedPoint={{ ...mockPoint, y: 123.45, count: 5 }}
        widgetId="w1"
        widget={mockWidget}
        dashboardName="dashboard1"
      />,
    );

    expect(
      screen.getByText((_content, element) => {
        return (
          element?.textContent === '5 samples have a mean of 123.45' &&
          element.tagName === 'SPAN'
        );
      }),
    ).toBeInTheDocument();
  });

  it('should display descriptive sentence in header (singular)', () => {
    mockUseFetchWidgetRawSamples.mockReturnValue(
      createMockQueryResult({ rows: [], nextPageToken: '' }),
    );
    render(
      <SampleDetailsContent
        selectedPoint={{ ...mockPoint, y: 123.45, count: 1 }}
        widgetId="w1"
        widget={mockWidget}
        dashboardName="dashboard1"
      />,
    );

    expect(
      screen.getByText((_content, element) => {
        return (
          element?.textContent === '1 sample has a mean of 123.45' &&
          element.tagName === 'SPAN'
        );
      }),
    ).toBeInTheDocument();
  });

  it('should hide details with empty string values', () => {
    mockUseFetchWidgetRawSamples.mockReturnValue(
      createMockQueryResult({
        nextPageToken: '',
        rows: [
          {
            values: {
              value: '100',
              test_name: 'test_class#test_method',
              atp_test_name: 'atp_test',
              build_id: '123',
              build_branch: 'main',
              build_target: 'target',
              invocation_complete_timestamp: '2026-04-04T10:00:00Z',
              empty_detail: '',
              filled_detail: 'filled',
            },
          },
        ],
      }),
    );

    render(
      <SampleDetailsContent
        selectedPoint={mockPoint}
        widgetId="w1"
        widget={mockWidget}
        dashboardName="dashboard1"
      />,
    );

    const accordion = screen.getByText(COMMON_MESSAGES.ALL_DETAILS);
    fireEvent.click(accordion);

    expect(screen.getByText('Filled Detail:')).toBeInTheDocument();
    expect(screen.queryByText('Empty Detail:')).not.toBeInTheDocument();
  });

  it('should call useFetchWidgetRawSamples with correct parameters', () => {
    mockUseFetchWidgetRawSamples.mockReturnValue(
      createMockQueryResult({ rows: [], nextPageToken: '' }),
    );
    render(
      <SampleDetailsContent
        selectedPoint={mockPoint}
        widgetId="w1"
        widget={mockWidget}
        dashboardName="dashboard1"
      />,
    );

    expect(mockUseFetchWidgetRawSamples).toHaveBeenCalledWith(
      expect.objectContaining({
        widgetId: 'w1',
        seriesId: 's1',
        pageSize: 1000,
        orderBy: 'value asc', // Default sort
      }),
    );
  });

  it('should handle sort direction change', () => {
    mockUseFetchWidgetRawSamples.mockReturnValue(
      createMockQueryResult({ rows: [], nextPageToken: '' }),
    );
    render(
      <SampleDetailsContent
        selectedPoint={mockPoint}
        widgetId="w1"
        widget={mockWidget}
        dashboardName="dashboard1"
      />,
    );

    const btn = screen.getByTestId('sort-direction-btn');
    fireEvent.click(btn);

    expect(mockUseFetchWidgetRawSamples).toHaveBeenLastCalledWith(
      expect.objectContaining({
        orderBy: 'value desc',
      }),
    );
  });
});
