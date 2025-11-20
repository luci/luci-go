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

import { render, screen } from '@testing-library/react';

import {
  TimeSeriesChart,
  TimeSeriesDefaultDataPoint,
} from './time_series_chart';

describe('TimeSeriesChart', () => {
  const mockData: TimeSeriesDefaultDataPoint[] = [
    { time: new Date('2025-10-30T00:00:00Z').getTime(), value: 100 },
    { time: new Date('2025-10-31T00:00:00Z').getTime(), value: 150 },
    { time: new Date('2025-11-01T00:00:00Z').getTime(), value: 120 },
  ];
  const mockMetricNameA = 'Mock Metric Name A';
  const mockMetricNameB = 'Mock Metric Name B';
  const mockTitle = 'Mock Chart Title';

  test('renders chart elements', () => {
    render(
      <TimeSeriesChart
        data={mockData}
        lines={[
          { dataKey: 'value', stroke: '#8884d8', name: mockMetricNameA },
          { dataKey: 'value', stroke: '#8884d8', name: mockMetricNameB },
        ]}
        chartTitle={mockTitle}
        xAxisDataKey="time"
        yAxisLabel="value"
        useResponsiveContainer={false}
      />,
    );

    // Check for legend item text and chart title
    expect(screen.getByText(mockMetricNameA)).toBeInTheDocument();
    expect(screen.getByText(mockMetricNameB)).toBeInTheDocument();
    expect(screen.getByText(mockTitle)).toBeInTheDocument();

    // Check if SVG is rendered
    const svgElement = document.querySelector('.recharts-surface');
    expect(svgElement).toBeInTheDocument();
  });
});
