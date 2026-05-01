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
 * Represents a single series to be plotted on the time series chart.
 */
export interface TimeSeriesDataSet {
  /**
   * The name of the series, used in the legend and tooltips.
   */
  name: string;
  /**
   * The data points for this series, where each point is an object.
   * 'x' should be a number (time or build ID).
   * 'y' should be a number.
   * 'count' is the number of aggregated rows.
   */
  data: Array<{
    x: number;
    y: number;
    count: number;
    point?: Record<string, unknown>;
    seriesId?: string;
    seriesIndex?: number;
  }>;
  /**
   * CSS color for the line.
   */
  stroke: string;
}

/**
 * Parameters for the chart tooltip, provided by ECharts.
 */
export interface ChartTooltipParam {
  axisValue: string | number;
  marker: string;
  seriesName: string;
  data: [number, number, number?];
}

/**
 * Parameters provided by the chart component when a data point is clicked.
 */
export interface PointClickParams {
  componentType: string;
  seriesName: string;
  data: [number, number, number, string?, string?, number?];
}

/**
 * Type guard to check if an object is PointClickParams.
 */
function isPointClickParams(params: unknown): params is PointClickParams {
  return (
    typeof params === 'object' &&
    params !== null &&
    'componentType' in params &&
    'seriesName' in params &&
    'data' in params
  );
}

function isChartTooltipParam(obj: unknown): obj is ChartTooltipParam {
  if (typeof obj !== 'object' || obj === null) return false;
  return (
    'axisValue' in obj &&
    'marker' in obj &&
    'seriesName' in obj &&
    'data' in obj &&
    Array.isArray(obj.data) &&
    obj.data.length >= 2
  );
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

  /**
   * Optional chart type for the series, defaults to 'line'.
   */
  chartType?: 'line' | 'scatter';
  /**
   * Optional custom tooltip formatter.
   */
  tooltipFormatter?: (
    params: ChartTooltipParam | ChartTooltipParam[],
  ) => string;
  /**
   * Optional explicit min value for X-axis.
   */
  xAxisMin?: number;
  /**
   * Optional explicit max value for X-axis.
   */
  xAxisMax?: number;
  /**
   * Optional callback when a data point is clicked.
   */
  onPointClick?: (params: PointClickParams) => void;
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
    show: false,
  },
  grid: {
    top: 40,
    left: 60,
    right: 30,
    bottom: 80,
    containLabel: true,
  },
  toolbox: {
    right: 20,
    top: 0,
    feature: {
      restore: {},
      saveAsImage: {},
      dataZoom: {},
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
      zoomOnMouseWheel: false,
      moveOnMouseMove: true,
    },
    {
      type: 'slider',
      yAxisIndex: 0,
      filterMode: 'none',
      left: 10,
      width: 20,
      orient: 'vertical',
    },
    {
      type: 'inside',
      yAxisIndex: 0,
      filterMode: 'none',
      zoomOnMouseWheel: false,
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
  chartType = 'line',
  tooltipFormatter,
  xAxisMin,
  xAxisMax,
  onPointClick,
}: TimeSeriesChartProps) {
  const theme = useTheme();

  const option: EChartsOption = useMemo(() => {
    const showDataZoom =
      xAxisMin !== undefined && xAxisMax !== undefined
        ? xAxisMin !== xAxisMax
        : (() => {
            let minX = Infinity;
            let maxX = -Infinity;
            series.forEach((s) => {
              s.data.forEach((pt) => {
                const x = pt.x;
                if (x < minX) minX = x;
                if (x > maxX) maxX = x;
              });
            });
            return minX !== maxX;
          })();

    const showYDataZoom = (() => {
      let minY = Infinity;
      let maxY = -Infinity;
      series.forEach((s) => {
        s.data.forEach((pt) => {
          const y = pt.y;
          if (y < minY) minY = y;
          if (y > maxY) maxY = y;
        });
      });
      return minY !== maxY;
    })();

    const filterMode = chartType === 'scatter' ? 'filter' : 'none';
    const dataZoomArray = Array.isArray(BASE_OPTION.dataZoom)
      ? BASE_OPTION.dataZoom
      : [];

    return {
      ...BASE_OPTION,
      dataZoom: [
        { ...(dataZoomArray[0] ?? {}), show: showDataZoom, filterMode },
        { ...(dataZoomArray[1] ?? {}), disabled: !showDataZoom, filterMode },
        { ...(dataZoomArray[2] ?? {}), show: showYDataZoom, filterMode },
        { ...(dataZoomArray[3] ?? {}), disabled: !showYDataZoom, filterMode },
      ],
      title: {
        ...BASE_OPTION.title,
        text: chartTitle,
        textStyle: {
          fontSize: theme.typography.subtitle1.fontSize,
          fontWeight: 'bold',
          color: theme.palette.text.primary,
        },
      },
      tooltip: {
        ...BASE_OPTION.tooltip,
        enterable: true,
        confine: true,
        extraCssText: 'max-height: 400px; overflow-y: auto;',
        formatter: tooltipFormatter
          ? (params: unknown) => {
              // ECharts passes dynamic data at runtime (e.g. data can be string, number, Date).
              // We use unknown and a type guard to safely narrow it to ChartTooltipParam.
              const rawItems = Array.isArray(params) ? params : [params];
              const validItems = rawItems.filter(isChartTooltipParam);
              if (validItems.length === 0) return '';
              return tooltipFormatter(
                Array.isArray(params) ? validItems : validItems[0],
              );
            }
          : undefined,
      },
      xAxis:
        xAxisType === 'time'
          ? {
              type: 'time',
              splitLine: { show: true, lineStyle: { type: 'dashed' } },
              axisLabel: {
                formatter: xAxisFormatter,
              },
              min: xAxisMin,
              max: xAxisMax,
            }
          : {
              type: 'value',
              splitLine: { show: true, lineStyle: { type: 'dashed' } },
              axisLabel: {
                formatter: (val: number) => val.toString(),
              },
              min: 'dataMin',
              max: 'dataMax',
            },
      yAxis: {
        ...BASE_OPTION.yAxis,
        name: yAxisLabel,
      },
      series: series.map((s) => {
        const common = {
          name: s.name,
          smooth: false,
          showSymbol: chartType === 'scatter' || s.data.length === 1,
          data: s.data.map((pt) => [
            pt.x,
            pt.y,
            pt.count,
            pt.point ? JSON.stringify(pt.point) : '',
            pt.seriesId,
            pt.seriesIndex,
          ]),
          itemStyle: { color: s.stroke },
          clip: true,
          valueFormatter: (val: number | string) => {
            if (typeof val === 'number') {
              return val.toLocaleString();
            }
            return val;
          },
        };
        return { ...common, type: chartType };
      }),
    };
  }, [
    series,
    chartTitle,
    yAxisLabel,
    xAxisType,
    theme,
    chartType,
    tooltipFormatter,
    xAxisMin,
    xAxisMax,
  ]);

  const onEvents = useMemo(
    () => ({
      click: (params: unknown) => {
        if (onPointClick && isPointClickParams(params)) {
          onPointClick(params);
        }
      },
    }),
    [onPointClick],
  );

  return (
    <Box
      data-testid="time-series-chart"
      sx={{
        position: 'relative',
        height: (theme) => theme.spacing(65),
        width: useResponsiveContainer ? '100%' : (theme) => theme.spacing(75),
        minWidth: 0,
        overflow: 'hidden',
      }}
    >
      <ReactECharts
        option={option}
        style={CHART_STYLE}
        notMerge={true}
        onEvents={onEvents}
      />
    </Box>
  );
}
