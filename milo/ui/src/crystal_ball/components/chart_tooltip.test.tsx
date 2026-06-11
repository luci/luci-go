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

import '@testing-library/jest-dom';

import { fireEvent, render, screen } from '@testing-library/react';

import {
  ChartTooltip,
  ChartTooltipParam,
  TimeSeriesDataSet,
} from '@/crystal_ball/components';
import { Column } from '@/crystal_ball/constants';

describe('ChartTooltip', () => {
  const mockSeries: TimeSeriesDataSet[] = [
    {
      name: 'Metric Series A',
      metricField: 'metric_key_a',
      data: [
        { x: 1716900000000, y: 100, count: 1 },
        { x: 1716903600000, y: 150, count: 2 },
      ],
      stroke: '#8884d8',
    },
    {
      name: 'Metric Series B',
      metricField: 'metric_key_b',
      data: [
        { x: 1716900000000, y: 200, count: 1 },
        { x: 1716903600000, y: 180, count: 5 },
      ],
      stroke: '#82ca9d',
    },
  ];

  const mockItems: ChartTooltipParam[] = [
    {
      axisValue: 1716903600000,
      marker: '<span style="background-color:#8884d8;"></span>',
      seriesName: 'Metric Series A',
      data: [
        1716903600000,
        150,
        2,
        JSON.stringify({ point_meta: 'point_a' }),
        's1',
        0,
      ],
    },
    {
      axisValue: 1716903600000,
      marker: '<span style="background-color:#82ca9d;"></span>',
      seriesName: 'Metric Series B',
      data: [
        1716903600000,
        180,
        5,
        JSON.stringify({ point_meta: 'point_b' }),
        's2',
        1,
      ],
    },
  ];

  const defaultProps = {
    items: mockItems,
    chartSeries: mockSeries,
    isDistribution: false,
    timeZone: 'UTC',
    xAxisDataKey: 'timestamp',
    onRowClick: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders nothing (null) when items list is empty', () => {
    const { container } = render(<ChartTooltip {...defaultProps} items={[]} />);
    expect(container.firstChild).toBeNull();
  });

  test('renders tooltip contents successfully for time axis', () => {
    render(<ChartTooltip {...defaultProps} />);

    // Verification description label is in document
    expect(screen.getByText('Click row for raw samples')).toBeInTheDocument();

    // Verify time is formatted correctly
    expect(screen.getByText(/2024-05-28/)).toBeInTheDocument();

    // Verify series names are rendered
    expect(screen.getByText('Metric Series A')).toBeInTheDocument();
    expect(screen.getByText('Metric Series B')).toBeInTheDocument();

    // Verify Y values are rendered with the aggregation sample count (n=X)
    expect(screen.getByText('150')).toBeInTheDocument();
    expect(screen.getByText('(n=2)')).toBeInTheDocument();
    expect(screen.getByText('180')).toBeInTheDocument();
    expect(screen.getByText('(n=5)')).toBeInTheDocument();
  });

  test('renders value axis header correctly without timestamp formatting when xAxisDataKey is BUILD_ID', () => {
    render(
      <ChartTooltip
        {...defaultProps}
        xAxisDataKey={Column.BUILD_ID}
        items={[
          {
            axisValue: 123456,
            marker: '<span></span>',
            seriesName: 'Metric Series A',
            data: [123456, 150, 1, '', 's1', 0],
          },
        ]}
      />,
    );

    // Verify raw value 123456 is rendered as the header
    expect(screen.getByText('123456')).toBeInTheDocument();
  });

  test('renders regression trend change badges for multi-metric chart', () => {
    render(<ChartTooltip {...defaultProps} />);

    // Metric Series A: base value is 100, target is 150.
    // Difference is +50 (+50.0%), trend is up
    expect(screen.getAllByText('+50 (+50.0%)')).toHaveLength(2);
    expect(screen.getAllByTestId('TrendingUpIcon')).toHaveLength(2);

    // Metric Series B: base value is 200, target is 180.
    // Difference is -20 (-10.0%), trend is down
    expect(screen.getAllByText('-20 (-10.0%)')).toHaveLength(2);
    expect(screen.getAllByTestId('TrendingDownIcon')).toHaveLength(2);
  });

  test('does NOT render regression trend change badges when isDistribution is true', () => {
    render(<ChartTooltip {...defaultProps} isDistribution={true} />);

    expect(screen.queryByText('+50 (+50.0%)')).not.toBeInTheDocument();
    expect(screen.queryByText('-20 (-10.0%)')).not.toBeInTheDocument();
  });

  test('fires onRowClick callback with correct parameters when a table row is clicked', () => {
    const onRowClickMock = jest.fn();
    render(<ChartTooltip {...defaultProps} onRowClick={onRowClickMock} />);

    // Click on the first row (Metric Series A)
    fireEvent.click(screen.getByText('Metric Series A'));

    expect(onRowClickMock).toHaveBeenCalledTimes(1);
    expect(onRowClickMock).toHaveBeenCalledWith(mockItems[0]);
  });
});
