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

import { Box, useTheme } from '@mui/material';
import { EChartsOption } from 'echarts';
import ReactECharts from 'echarts-for-react';
import { CSSProperties, useMemo } from 'react';

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
 * Represents a single series to be plotted on the time series chart.
 */
export interface TimeSeriesDataSet {
  /**
   * The name of the series, used in the legend and tooltips.
   */
  name: string;
  /**
   * The data points for this series, where each tuple is [time, value].
   * 'time' should be a Unix timestamp (number).
   * 'value' should be a number.
   */
  data: Array<[number, number]>;
  /**
   * CSS color for the line.
   */
  stroke: string;
}

/**
 * Props for the TimeSeriesChart.
 */
interface TimeSeriesChartProps {
  /**
   * An array of datasets to render on the time series chart.
   * Each element represents a distinct line.
   */
  series: TimeSeriesDataSet[];

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

  /**
   * Optional X-axis type for the chart, defaults to 'time'.
   */
  xAxisType?: 'time' | 'value';
}

/**
 * Format large numbers for display on the Y-axis and tooltips.
 * @param num - any number value to be formatted.
 * @returns a formatted number display string.
 */
function formatLargeNumber(num: number): string {
  if (num === 0) return '0';
  if (Math.abs(num) < 1000) return num.toString();

  const units = ['', 'k', 'M', 'B', 'T'];
  const order = Math.floor(Math.log10(Math.abs(num)) / 3);
  const name = units[Math.min(order, units.length - 1)];
  const value = Math.abs(num) / Math.pow(1000, order);

  return `${Math.sign(num) * parseFloat(value.toFixed(1))}${name}`;
}

/**
 * Formats a number for the X-axis time display.
 */
function xAxisFormatter(val: number) {
  const date = new Date(val);
  return `${date.toLocaleDateString()}\n${date.toLocaleTimeString(
    /* locales= */ undefined,
    {
      hour: '2-digit',
      minute: '2-digit',
    },
  )}`;
}

const BASE_OPTION: Partial<EChartsOption> = {
  title: {
    left: 'left',
    top: 0,
    textStyle: {
      fontSize: 16,
      fontWeight: 'bold',
    },
  },
  tooltip: {
    trigger: 'axis',
    axisPointer: {
      type: 'cross',
    },
    appendToBody: true,
  },
  legend: {
    bottom: 0,
    type: 'scroll',
  },
  grid: {
    top: 40,
    left: 20,
    right: 60,
    bottom: 80,
    containLabel: true,
  },
  toolbox: {
    right: 20,
    top: 0,
    feature: {
      restore: {},
      saveAsImage: {},
    },
  },
  dataZoom: [
    {
      type: 'slider',
      xAxisIndex: 0,
      filterMode: 'none',
      bottom: 30,
      height: 20,
    },
    {
      type: 'inside',
      xAxisIndex: 0,
      filterMode: 'none',
      zoomOnMouseWheel: true,
      moveOnMouseMove: true,
    },
  ],
  xAxis: {
    type: 'time',
    splitLine: { show: true, lineStyle: { type: 'dashed' } },
    axisLabel: {
      formatter: xAxisFormatter,
    },
  },
  yAxis: {
    type: 'value',
    nameLocation: 'middle',
    nameGap: 30,
    axisLabel: { formatter: formatLargeNumber },
    splitLine: { show: true, lineStyle: { type: 'dashed' } },
  },
};

const CHART_STYLE: CSSProperties = {
  position: 'absolute',
  top: 0,
  left: 0,
  height: '100%',
  width: '100%',
};

/**
 * Uses echarts to render a time series chart.
 */
export function TimeSeriesChart({
  series,
  yAxisLabel,
  chartTitle,
  useResponsiveContainer = true,
  xAxisType = 'time',
}: TimeSeriesChartProps) {
  const theme = useTheme();

  const option: EChartsOption = useMemo(() => {
    return {
      ...BASE_OPTION,
      title: {
        ...BASE_OPTION.title,
        text: chartTitle,
        textStyle: {
          fontSize: theme.typography.subtitle1.fontSize,
          fontWeight: 'bold',
          color: theme.palette.text.primary,
        },
      },
      xAxis:
        xAxisType === 'time'
          ? {
              type: 'time',
              splitLine: { show: true, lineStyle: { type: 'dashed' } },
              axisLabel: {
                formatter: xAxisFormatter,
              },
            }
          : {
              type: 'value',
              splitLine: { show: true, lineStyle: { type: 'dashed' } },
              min: 'dataMin',
              max: 'dataMax',
            },
      yAxis: {
        ...BASE_OPTION.yAxis,
        name: yAxisLabel,
      },
      series: series.map((s) => ({
        name: s.name,
        type: 'line',
        smooth: false,
        showSymbol: false,
        data: s.data,
        itemStyle: { color: s.stroke },
        valueFormatter: (val: number | string) => {
          if (typeof val === 'number') {
            return val.toLocaleString();
          }
          return val;
        },
      })),
    };
  }, [series, chartTitle, yAxisLabel, xAxisType, theme]);

  return (
    <Box
      data-testid="time-series-chart"
      sx={{
        position: 'relative',
        height: `${DEFAULT_LINE_CHART_HEIGHT_PX}px`,
        width: useResponsiveContainer
          ? '100%'
          : `${DEFAULT_LINE_CHART_WIDTH_PX}px`,
        minWidth: 0,
        overflow: 'hidden',
      }}
    >
      <ReactECharts option={option} style={CHART_STYLE} />
    </Box>
  );
}
