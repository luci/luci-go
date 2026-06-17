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
import { ComponentProps, type CSSProperties, type Ref } from 'react';

import { TimeSeriesChart, TimeSeriesDataSet } from '@/crystal_ball/components';

const mockDispatchAction = jest.fn();
const mockGetOption = jest.fn(() => ({
  dataZoom: [{ startValue: 10, endValue: 20 }],
}));
const mockGetDataURL = jest.fn(() => 'data:image/png;base64,mock-data-url');
const mockOn = jest.fn();
const mockOff = jest.fn();
const mockSetCursorStyle = jest.fn();
const mockGetZr = jest.fn(() => ({
  setCursorStyle: mockSetCursorStyle,
}));

const mockEchartsInstance = {
  dispatchAction: mockDispatchAction,
  getOption: mockGetOption,
  getDataURL: mockGetDataURL,
  on: mockOn,
  off: mockOff,
  getZr: mockGetZr,
};

const mockGetEchartsInstance = jest.fn(() => mockEchartsInstance);

jest.mock('echarts-for-react', () => {
  const ReactActual: typeof import('react') = jest.requireActual('react');
  const MockReactEChartsComponent = (props: MockEChartsProps) => {
    ReactActual.useImperativeHandle(
      props.ref,
      () => ({
        getEchartsInstance: mockGetEchartsInstance,
      }),
      [props.ref],
    );

    return <canvas data-testid="echarts-canvas" style={props.style} />;
  };
  MockReactEChartsComponent.displayName = 'MockReactEChartsComponent';
  return jest.fn(MockReactEChartsComponent);
});

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
  ref?: Ref<unknown>;
}

const MockECharts = jest.mocked(ReactECharts);

describe('TimeSeriesChart', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockDispatchAction.mockClear();
    mockGetOption.mockClear();
    mockGetDataURL.mockClear();
    mockGetZr.mockClear();
    mockSetCursorStyle.mockClear();
  });

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
      10,
    ]);
  });

  test('respects fitY prop', () => {
    const { rerender } = render(
      <TimeSeriesChart series={mockSeries} fitY={true} />,
    );

    let lastCallProps = MockECharts.mock.lastCall![0];
    expect(lastCallProps.option.yAxis).toBeDefined();
    expect(lastCallProps.option.yAxis?.scale).toBe(true);
    expect(lastCallProps.option.yAxis?.min).toBeUndefined();

    rerender(<TimeSeriesChart series={mockSeries} fitY={false} />);

    lastCallProps = MockECharts.mock.lastCall![0];
    expect(lastCallProps.option.yAxis?.scale).toBe(false);
    expect(lastCallProps.option.yAxis?.min).toBe(0);
  });

  test('dispatches takeGlobalCursor action when isZoomActive is toggled', () => {
    const { rerender } = render(
      <TimeSeriesChart series={mockSeries} isZoomActive={true} />,
    );

    expect(mockDispatchAction).toHaveBeenCalledWith({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: {
        brushType: 'lineX',
        brushMode: 'single',
      },
    });

    mockDispatchAction.mockClear();
    mockSetCursorStyle.mockClear();

    rerender(<TimeSeriesChart series={mockSeries} isZoomActive={false} />);

    expect(mockDispatchAction).toHaveBeenCalledWith({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: {
        brushType: 'none',
        brushMode: 'single',
      },
    });
    expect(mockDispatchAction).toHaveBeenCalledWith({
      type: 'brush',
      areas: [],
    });
    expect(mockGetZr).toHaveBeenCalledTimes(1);
    expect(mockSetCursorStyle).toHaveBeenCalledWith('default');
  });

  test('dispatches dataZoom range reset when restoreZoomTrigger changes', () => {
    const { rerender } = render(
      <TimeSeriesChart series={mockSeries} restoreZoomTrigger={0} />,
    );

    expect(mockDispatchAction).not.toHaveBeenCalledWith({
      type: 'dataZoom',
      dataZoomIndex: 0,
      start: 0,
      end: 100,
    });

    rerender(<TimeSeriesChart series={mockSeries} restoreZoomTrigger={1} />);

    expect(mockDispatchAction).toHaveBeenCalledWith({
      type: 'dataZoom',
      dataZoomIndex: 0,
      start: 0,
      end: 100,
    });
    expect(mockDispatchAction).toHaveBeenCalledWith({
      type: 'dataZoom',
      dataZoomIndex: 1,
      start: 0,
      end: 100,
    });
  });

  test('triggers chart download when downloadTrigger changes', () => {
    const clickSpy = jest
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => {});

    const { rerender } = render(
      <TimeSeriesChart
        series={mockSeries}
        downloadTrigger={0}
        chartTitle="Test Title"
      />,
    );

    expect(mockGetDataURL).not.toHaveBeenCalled();
    expect(clickSpy).not.toHaveBeenCalled();

    rerender(
      <TimeSeriesChart
        series={mockSeries}
        downloadTrigger={1}
        chartTitle="Test Title"
      />,
    );

    expect(mockGetDataURL).toHaveBeenCalledWith({
      type: 'png',
      pixelRatio: 2,
      backgroundColor: expect.any(String),
    });
    expect(clickSpy).toHaveBeenCalledTimes(1);

    clickSpy.mockRestore();
  });

  test('updates zoom range and dispatches actions on brushEnd with flat areas', () => {
    render(<TimeSeriesChart series={mockSeries} />);

    const lastCallProps = MockECharts.mock.lastCall![0];
    const onEvents = lastCallProps.onEvents;

    expect(onEvents).toBeDefined();
    expect(onEvents!.brushEnd).toBeDefined();

    const mockBrushParams = {
      areas: [
        {
          coordRange: [1000, 2000],
        },
      ],
    };

    onEvents!.brushEnd(mockBrushParams);

    expect(mockDispatchAction).toHaveBeenCalledWith({
      type: 'dataZoom',
      dataZoomIndex: 0,
      startValue: 1000,
      endValue: 2000,
    });

    expect(mockDispatchAction).toHaveBeenCalledWith({
      type: 'brush',
      areas: [],
    });
  });

  test('updates zoom range and dispatches actions on brushEnd with batch-nested areas', () => {
    render(<TimeSeriesChart series={mockSeries} />);

    const lastCallProps = MockECharts.mock.lastCall![0];
    const onEvents = lastCallProps.onEvents;

    expect(onEvents).toBeDefined();
    expect(onEvents!.brushEnd).toBeDefined();

    const mockBrushParams = {
      batch: [
        {
          areas: [
            {
              coordRange: [1500, 2500],
            },
          ],
        },
      ],
    };

    onEvents!.brushEnd(mockBrushParams);

    expect(mockDispatchAction).toHaveBeenCalledWith({
      type: 'dataZoom',
      dataZoomIndex: 0,
      startValue: 1500,
      endValue: 2500,
    });

    expect(mockDispatchAction).toHaveBeenCalledWith({
      type: 'brush',
      areas: [],
    });
  });

  test('persists zoom range on next render', () => {
    const { rerender } = render(<TimeSeriesChart series={mockSeries} />);

    const lastCallProps = MockECharts.mock.lastCall![0];
    const onEvents = lastCallProps.onEvents;

    // Trigger datazoom to update the silent persistence ref
    mockGetOption.mockReturnValueOnce({
      dataZoom: [{ startValue: 1200, endValue: 1800 }],
    });
    onEvents!.datazoom();

    // Rerender the chart (e.g. state change)
    rerender(<TimeSeriesChart series={mockSeries} fitY={true} />);

    // Verify that the new options include the persisted startValue and endValue!
    const newCallProps = MockECharts.mock.lastCall![0];
    expect(newCallProps.option.dataZoom?.[0]).toMatchObject({
      startValue: 1200,
      endValue: 1800,
    });
    expect(newCallProps.option.dataZoom?.[1]).toMatchObject({
      startValue: 1200,
      endValue: 1800,
    });
  });
});
