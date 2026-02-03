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
        [new Date('2025-10-30T00:00:00Z').getTime(), 100],
        [new Date('2025-10-31T00:00:00Z').getTime(), 150],
        [new Date('2025-11-01T00:00:00Z').getTime(), 120],
      ],
      stroke: '#8884d8',
    },
    {
      name: mockMetricNameB,
      data: [
        [new Date('2025-10-30T00:00:00Z').getTime(), 100],
        [new Date('2025-10-31T00:00:00Z').getTime(), 150],
        [new Date('2025-11-01T00:00:00Z').getTime(), 120],
      ],
      stroke: '#82ca9d',
    },
  ];

  test('renders chart canvas successfully with correct config', () => {
    render(<TimeSeriesChart series={mockSeries} />);

    const canvasElement = screen.getByTestId('echarts-canvas');
    expect(canvasElement).toBeInTheDocument();

    const lastCallProps: MockEChartsProps = MockECharts.mock.lastCall![0];
    const { option } = lastCallProps;

    expect(canvasElement).toHaveStyle({ height: '400px', width: '100%' });

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
});
