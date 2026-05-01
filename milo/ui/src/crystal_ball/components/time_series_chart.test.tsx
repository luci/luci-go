// Copyright 2025 The LUCI Authors.
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

import { jest } from '@jest/globals';
import { render, screen } from '@testing-library/react';
import ReactECharts from 'echarts-for-react';
import { ComponentProps, type CSSProperties } from 'react';

import { TimeSeriesChart, TimeSeriesDataSet } from '@/crystal_ball/components';

jest.mock('echarts-for-react', () =>
  jest.fn((props: MockEChartsProps) => (
    <canvas data-testid="echarts-canvas" style={props.style} />
  )),
);

// We override 'option' to match the specific shape used in our tests.
interface MockEChartsProps
  extends Omit<ComponentProps<typeof ReactECharts>, 'option' | 'style'> {
  option: {
    legend: {
      data?: string[];
    };
    series: {
      name: string;
      clip?: boolean;
      data?: Array<[number, number, number, string?, string?, number?]>;
    }[];
    xAxis?: {
      type: string;
      min?: number;
      max?: number;
    };
    dataZoom?: {
      show?: boolean;
      disabled?: boolean;
      yAxisIndex?: number;
      orient?: string;
      left?: number | string;
      filterMode?: string;
    }[];
  };
  style?: CSSProperties;
}

const MockECharts = jest.mocked(ReactECharts);

describe('TimeSeriesChart', () => {
  const mockMetricNameA = 'Mock Metric Name A';
  const mockMetricNameB = 'Mock Metric Name B';

  const mockSeries: TimeSeriesDataSet[] = [
    {
      name: mockMetricNameA,
      data: [
        { x: new Date('2025-10-30T00:00:00Z').getTime(), y: 100, count: 1 },
        { x: new Date('2025-10-31T00:00:00Z').getTime(), y: 150, count: 1 },
        { x: new Date('2025-11-01T00:00:00Z').getTime(), y: 120, count: 1 },
      ],
      stroke: '#8884d8',
    },
    {
      name: mockMetricNameB,
      data: [
        { x: new Date('2025-10-30T00:00:00Z').getTime(), y: 100, count: 1 },
        { x: new Date('2025-10-31T00:00:00Z').getTime(), y: 150, count: 1 },
        { x: new Date('2025-11-01T00:00:00Z').getTime(), y: 120, count: 1 },
      ],
      stroke: '#82ca9d',
    },
  ];

  test('renders chart canvas successfully with correct config', () => {
    render(
      <TimeSeriesChart series={mockSeries} useResponsiveContainer={false} />,
    );

    const canvasElement = screen.getByTestId('echarts-canvas');
    expect(canvasElement).toBeInTheDocument();

    const lastCallProps: MockEChartsProps = MockECharts.mock.lastCall![0];
    const { option } = lastCallProps;

    expect(canvasElement).toHaveStyle({ height: '100%', width: '100%' });

    // We cannot directly test if the metric name is in the canvas because it renders
    // pixels. So we check if it's correctly set in the options, and rely on the
    // libraries correctness.
    const seriesNames = option.series.map((s) => s.name);
    expect(seriesNames).toContain(mockMetricNameA);
    expect(seriesNames).toContain(mockMetricNameB);
  });

  test('renders without crashing when data is empty', () => {
    render(<TimeSeriesChart series={[]} />);

    expect(screen.getByTestId('echarts-canvas')).toBeInTheDocument();
  });

  test('renders with value xAxisType when specified', () => {
    render(
      <TimeSeriesChart
        series={mockSeries}
        useResponsiveContainer={false}
        xAxisType="value"
      />,
    );

    expect(screen.getByTestId('echarts-canvas')).toBeInTheDocument();

    const lastCallProps = MockECharts.mock.lastCall![0];
    const option = lastCallProps.option;

    expect(option.xAxis).toBeDefined();
    expect(option.xAxis?.type).toBe('value');
  });

  test('hides dataZoom when there is only 1 unique X value', () => {
    const singlePointSeries: TimeSeriesDataSet[] = [
      {
        name: mockMetricNameA,
        data: [{ x: 1000, y: 10, count: 1 }],
        stroke: '#8884d8',
      },
      {
        name: mockMetricNameB,
        data: [{ x: 1000, y: 20, count: 1 }],
        stroke: '#82ca9d',
      },
    ];

    render(<TimeSeriesChart series={singlePointSeries} />);

    const lastCallProps = MockECharts.mock.lastCall![0];
    const option = lastCallProps.option;

    expect(option.dataZoom).toBeDefined();
    expect(option.dataZoom?.[0]?.show).toBe(false);
    expect(option.dataZoom?.[1]?.disabled).toBe(true);
  });

  test('respects xAxisMin and xAxisMax props', () => {
    const mockSeries: TimeSeriesDataSet[] = [
      {
        name: 'Metric A',
        data: [{ x: 1000, y: 10, count: 1 }],
        stroke: '#8884d8',
      },
    ];

    render(
      <TimeSeriesChart series={mockSeries} xAxisMin={500} xAxisMax={1500} />,
    );

    const lastCallProps = MockECharts.mock.lastCall![0];
    const option = lastCallProps.option;

    expect(option.xAxis).toBeDefined();
    expect(option.xAxis?.min).toBe(500);
    expect(option.xAxis?.max).toBe(1500);
  });

  test('shows Y-axis dataZoom when Y values vary', () => {
    render(<TimeSeriesChart series={mockSeries} />);

    const lastCallProps = MockECharts.mock.lastCall![0];
    const option = lastCallProps.option;

    expect(option.dataZoom).toBeDefined();
    expect(option.dataZoom?.[2]?.show).toBe(true);
    expect(option.dataZoom?.[2]?.orient).toBe('vertical');
    expect(option.dataZoom?.[2]?.left).toBe(10);
    expect(option.dataZoom?.[3]?.disabled).toBe(false);
  });

  test('hides Y-axis dataZoom when Y values are identical', () => {
    const flatSeries: TimeSeriesDataSet[] = [
      {
        name: mockMetricNameA,
        data: [
          { x: 1000, y: 100, count: 1 },
          { x: 2000, y: 100, count: 1 },
        ],
        stroke: '#8884d8',
      },
    ];

    render(<TimeSeriesChart series={flatSeries} />);

    const lastCallProps = MockECharts.mock.lastCall![0];
    const option = lastCallProps.option;

    expect(option.dataZoom).toBeDefined();
    expect(option.dataZoom?.[2]?.show).toBe(false);
    expect(option.dataZoom?.[3]?.disabled).toBe(true);
  });

  test('sets filterMode to filter and clip to true for scatter chart', () => {
    render(<TimeSeriesChart series={mockSeries} chartType="scatter" />);

    const lastCallProps = MockECharts.mock.lastCall![0];
    const option = lastCallProps.option;

    expect(option.dataZoom).toBeDefined();
    expect(option.dataZoom?.[0]?.filterMode).toBe('filter');
    expect(option.dataZoom?.[1]?.filterMode).toBe('filter');
    expect(option.dataZoom?.[2]?.filterMode).toBe('filter');
    expect(option.dataZoom?.[3]?.filterMode).toBe('filter');

    expect(option.series).toBeDefined();
    expect(option.series[0]?.clip).toBe(true);
  });

  test('calls onPointClick when a point is clicked', () => {
    const mockOnPointClick = jest.fn();
    render(
      <TimeSeriesChart series={mockSeries} onPointClick={mockOnPointClick} />,
    );

    const lastCallProps = MockECharts.mock.lastCall![0];
    const onEvents = lastCallProps.onEvents;

    expect(onEvents).toBeDefined();
    expect(onEvents!.click).toBeDefined();

    const mockParams = {
      componentType: 'series',
      seriesName: 'mock_series',
      data: [1000, 50, 1],
    };
    onEvents!.click(mockParams);

    expect(mockOnPointClick).toHaveBeenCalledWith(mockParams);
  });

  test('passes extra data fields to ECharts series data', () => {
    const mockSeriesWithExtras: TimeSeriesDataSet[] = [
      {
        name: 'Metric A',
        data: [
          {
            x: 1000,
            y: 10,
            count: 1,
            point: { foo: 'bar' },
            seriesId: 's1',
            seriesIndex: 0,
          },
        ],
        stroke: '#8884d8',
      },
    ];

    render(<TimeSeriesChart series={mockSeriesWithExtras} />);

    const lastCallProps = MockECharts.mock.lastCall![0];
    const option = lastCallProps.option;

    expect(option.series).toBeDefined();
    const seriesData = option.series[0]?.data;
    expect(seriesData).toBeDefined();
    expect(seriesData![0]).toEqual([
      1000,
      10,
      1,
      JSON.stringify({ foo: 'bar' }),
      's1',
      0,
    ]);
  });
});
