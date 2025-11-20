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

import { Typography } from '@mui/material';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts';

/**
 * When not using a responsive container, the line chart height will fall back
 * to this setting.
 */
const DEFAULT_LINE_CHART_HEIGHT_PX = 400;

/**
 * When not using a responsive container, the line chart width will fall back
 * to this setting.
 */
const DEFAULT_LINE_CHART_WIDTH_PX = 600;

/**
 * The line chart supports custom x-axis and y-axis data accessor keys. By
 * default, the line chart will use "time" as the x-axis data key and "value" as
 * the y-axis data key.
 */
export interface TimeSeriesDefaultDataPoint {
  /**
   * Default x-axis data accessor key.
   */
  time: number;

  /**
   * Default y-axis data accessor key.
   */
  value: number;
}

/**
 * Specify the y-axis data accessor key and stroke color for a line to be
 * rendered on the time series chart.
 */
export interface TimeSeriesLine {
  /**
   * The y-axis data accessor key.
   */
  dataKey: string;

  /**
   * CSS color for the line.
   */
  stroke: string;

  /**
   * Optional name for the line.
   */
  name?: string;
}

/**
 * Props for the TimeSeriesChart.
 */
interface TimeSeriesChartProps<TDataPoint> {
  /**
   * List of generic data points for the line data. By default,
   * TimeSeriesDefaultDataPoint can be used as the generic interface.
   */
  data: TDataPoint[];

  /**
   * List of lines to render on the time series chart.
   */
  lines: TimeSeriesLine[];

  /**
   * Data key for accessing x-axis data.
   */
  xAxisDataKey: string;

  /**
   * Optional label string for the y-axis.
   */
  yAxisLabel?: string;

  /**
   * Optional title for the time series chart.
   */
  chartTitle?: string;

  /**
   * Optional flag for using a ResponsiveContainer, defaults to true.
   */
  useResponsiveContainer?: boolean;
}

export function TimeSeriesChart<TDataPoint>({
  data,
  lines,
  xAxisDataKey,
  yAxisLabel,
  chartTitle,
  useResponsiveContainer = true,
}: TimeSeriesChartProps<TDataPoint>) {
  let timeSeriesChart = (
    <LineChart
      data={data}
      // Default for height and width is undefined, and only need to be
      // specified when not using a responsive container.
      height={useResponsiveContainer ? undefined : DEFAULT_LINE_CHART_HEIGHT_PX}
      width={useResponsiveContainer ? undefined : DEFAULT_LINE_CHART_WIDTH_PX}
      margin={{
        top: 5,
        right: 30,
        left: 20,
        bottom: 5,
      }}
    >
      <CartesianGrid strokeDasharray="3 3" />
      <XAxis
        dataKey={xAxisDataKey}
        type="number"
        domain={['dataMin', 'dataMax']}
        tickFormatter={(unixTime) => new Date(unixTime).toLocaleDateString()}
        name="Time"
      />
      <YAxis
        label={{ value: yAxisLabel, angle: -90, position: 'insideLeft' }}
      />
      <Tooltip
        labelFormatter={(unixTime) => new Date(unixTime).toLocaleString()}
      />
      <Legend />
      {lines.map((line) => (
        <Line
          key={line.dataKey}
          type="monotone"
          dataKey={line.dataKey}
          stroke={line.stroke}
          name={line.name || line.dataKey}
          dot={false}
        />
      ))}
    </LineChart>
  );

  // By default, wrap the chart in a ResponsiveContainer that adjusts width to
  // 100%.
  if (useResponsiveContainer) {
    timeSeriesChart = (
      <ResponsiveContainer width="100%" height={400}>
        {timeSeriesChart}
      </ResponsiveContainer>
    );
  }

  return (
    <>
      {chartTitle && (
        <Typography variant="h6" component="h2" align="center" gutterBottom>
          {chartTitle}
        </Typography>
      )}
      {timeSeriesChart}
    </>
  );
}
