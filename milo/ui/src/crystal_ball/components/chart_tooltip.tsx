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

import {
  TrendingDown as TrendingDownIcon,
  TrendingFlat as TrendingFlatIcon,
  TrendingUp as TrendingUpIcon,
} from '@mui/icons-material';
import { alpha, Box, Typography } from '@mui/material';
import React from 'react';

import {
  ChartTooltipParam,
  TimeSeriesDataSet,
} from '@/crystal_ball/components';
import { Column } from '@/crystal_ball/constants';
import {
  calculateChange,
  formatChange,
  formatTimestampWithZone,
  getTrendInfo,
} from '@/crystal_ball/utils';

export interface ChartTooltipProps {
  items: readonly ChartTooltipParam[];
  chartSeries: readonly TimeSeriesDataSet[];
  isDistribution: boolean;
  timeZone: string;
  xAxisDataKey: string | undefined;
  onRowClick: (item: ChartTooltipParam) => void;
}

export function ChartTooltip({
  items,
  chartSeries,
  isDistribution,
  timeZone,
  xAxisDataKey,
  onRowClick,
}: ChartTooltipProps) {
  if (items.length === 0) return null;

  const firstItem = items[0];
  const xVal = firstItem.axisValue;
  let xDisplay = xVal;

  const xAxisType = xAxisDataKey === Column.BUILD_ID ? 'value' : 'time';
  if (xAxisType === 'time' && typeof xVal === 'number') {
    xDisplay = formatTimestampWithZone(xVal, timeZone);
  }

  return (
    <Box
      sx={{
        fontFamily: 'inherit',
        p: '6px',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          mb: 1,
          pb: 0.75,
          borderBottom: '1px solid',
          borderColor: 'divider',
        }}
      >
        <Typography
          variant="body2"
          sx={{
            fontWeight: 'bold',
            color: 'text.primary',
          }}
        >
          {xDisplay}
        </Typography>
        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
            fontWeight: 500,
            fontSize: '10px',
          }}
        >
          Click row for raw samples
        </Typography>
      </Box>
      <Box
        component="table"
        sx={{
          width: '100%',
          borderCollapse: 'collapse',
          fontSize: '12px',
        }}
      >
        <Box component="tbody">
          {items.map((item, idx) => {
            const val = item.data[1];
            const count = item.data[2];

            const currentSeries = chartSeries.find(
              (s) => s.name === item.seriesName,
            );
            let changeBadge = null;

            if (
              !isDistribution &&
              currentSeries &&
              currentSeries.data &&
              currentSeries.data.length > 0
            ) {
              const scale = currentSeries.yScaleFactor ?? 1;
              const { diff, pctChange } = calculateChange(
                currentSeries.data[0].y * scale,
                val,
              );

              if (Math.abs(diff) >= 0.0001) {
                const trendInfo = getTrendInfo(diff, currentSeries.metricField);
                const text = formatChange(diff, pctChange);

                const icon =
                  trendInfo.trend === 'up' ? (
                    <TrendingUpIcon fontSize="small" sx={{ mr: 0.5 }} />
                  ) : trendInfo.trend === 'down' ? (
                    <TrendingDownIcon fontSize="small" sx={{ mr: 0.5 }} />
                  ) : (
                    <TrendingFlatIcon fontSize="small" sx={{ mr: 0.5 }} />
                  );

                changeBadge = (
                  <Box
                    sx={{
                      bgcolor: (theme) =>
                        trendInfo.color === 'error.main'
                          ? alpha(theme.palette.error.main, 0.08)
                          : trendInfo.color === 'success.main'
                            ? alpha(theme.palette.success.main, 0.08)
                            : 'action.hover',
                      border: '1px solid',
                      borderColor:
                        trendInfo.color === 'error.main'
                          ? 'error.light'
                          : trendInfo.color === 'success.main'
                            ? 'success.light'
                            : 'divider',
                      borderRadius: '10px',
                      px: 0.75,
                      py: 0.15,
                      display: 'inline-flex',
                      alignItems: 'center',
                      fontSize: '10px',
                      color: trendInfo.color,
                      fontWeight: 'bold',
                      whiteSpace: 'nowrap',
                      mr: 0.5,
                    }}
                    title="Change vs baseline (first point)"
                  >
                    {icon}
                    {text}
                  </Box>
                );
              }

              const currentIndex = currentSeries.data.findIndex(
                (p) => p.x === item.data[0],
              );

              if (currentIndex > 0) {
                const prevY = currentSeries.data[currentIndex - 1].y * scale;
                const { diff: prevDiff, pctChange: prevPctChange } =
                  calculateChange(prevY, val);

                if (Math.abs(prevDiff) >= 0.0001) {
                  const trendInfo = getTrendInfo(
                    prevDiff,
                    currentSeries.metricField,
                  );
                  const text = formatChange(prevDiff, prevPctChange);

                  const icon =
                    trendInfo.trend === 'up' ? (
                      <TrendingUpIcon fontSize="small" sx={{ mr: 0.5 }} />
                    ) : trendInfo.trend === 'down' ? (
                      <TrendingDownIcon fontSize="small" sx={{ mr: 0.5 }} />
                    ) : (
                      <TrendingFlatIcon fontSize="small" sx={{ mr: 0.5 }} />
                    );

                  const prevChangeBadge = (
                    <Box
                      sx={{
                        bgcolor: (theme) =>
                          trendInfo.color === 'error.main'
                            ? alpha(theme.palette.error.main, 0.08)
                            : trendInfo.color === 'success.main'
                              ? alpha(theme.palette.success.main, 0.08)
                              : 'action.hover',
                        border: '1px solid',
                        borderColor:
                          trendInfo.color === 'error.main'
                            ? 'error.light'
                            : trendInfo.color === 'success.main'
                              ? 'success.light'
                              : 'divider',
                        borderRadius: '10px',
                        px: 0.75,
                        py: 0.15,
                        display: 'inline-flex',
                        alignItems: 'center',
                        fontSize: '10px',
                        color: trendInfo.color,
                        fontWeight: 'bold',
                        whiteSpace: 'nowrap',
                      }}
                      title="Change vs preceding point"
                    >
                      {icon}
                      {text}
                    </Box>
                  );

                  changeBadge = (
                    <>
                      {changeBadge}
                      {prevChangeBadge}
                    </>
                  );
                }
              }
            }

            const handleRowClick = (e: React.MouseEvent) => {
              e.preventDefault();
              onRowClick(item);
            };

            return (
              <Box
                component="tr"
                key={`${item.seriesName}-${idx}`}
                onClick={handleRowClick}
                sx={{
                  height: 24,
                  verticalAlign: 'middle',
                  cursor: 'pointer',
                  '&:hover': {
                    bgcolor: 'action.hover',
                  },
                  '&:active': {
                    bgcolor: 'action.selected',
                  },
                }}
              >
                <Box
                  component="td"
                  sx={{
                    py: 0.25,
                    pr: 1.5,
                    pl: 0,
                    whiteSpace: 'nowrap',
                    color: 'text.secondary',
                    fontWeight: 'bold',
                    minWidth: 140,
                    display: 'flex',
                    alignItems: 'center',
                  }}
                >
                  <Box
                    sx={{
                      display: 'inline-block',
                      mr: 0.5,
                      borderRadius: '50%',
                      width: 10,
                      height: 10,
                      bgcolor: currentSeries?.stroke || 'text.secondary',
                      border: '1px solid',
                      borderColor: 'divider',
                    }}
                  />
                  {item.seriesName}
                </Box>
                <Box
                  component="td"
                  sx={{
                    textAlign: 'right',
                    fontWeight: 'bold',
                    py: 0.25,
                    px: 1.5,
                    color: 'text.primary',
                    minWidth: 90,
                  }}
                >
                  {val.toLocaleString()}
                  {count !== undefined && count !== 0 && (
                    <Box
                      component="span"
                      sx={{
                        color: 'text.disabled',
                        fontSize: '10px',
                        fontWeight: 'normal',
                        fontStyle: 'italic',
                        ml: 0.5,
                      }}
                    >
                      (n={count})
                    </Box>
                  )}
                </Box>
                {!isDistribution && (
                  <Box
                    component="td"
                    sx={{
                      textAlign: 'right',
                      py: 0.25,
                      pl: 1.5,
                      pr: 0,
                      minWidth: 100,
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {changeBadge}
                  </Box>
                )}
              </Box>
            );
          })}
        </Box>
      </Box>
    </Box>
  );
}
