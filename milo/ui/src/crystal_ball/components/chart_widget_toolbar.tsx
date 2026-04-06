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
  Functions as FunctionsIcon,
  GroupWork as GroupWorkIcon,
  ScatterPlot as ScatterPlotIcon,
  ShowChart as ShowChartIcon,
} from '@mui/icons-material';
import {
  Box,
  Divider,
  FormControl,
  MenuItem,
  Select,
  SelectChangeEvent,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';

import {
  COMMON_MESSAGES,
  AGGREGATION_FUNCTION_LABELS,
  GROUP_BY_OPTIONS,
} from '@/crystal_ball/constants';
import { COMPACT_ICON_SX, COMPACT_SELECT_SX } from '@/crystal_ball/styles';
import {
  PerfChartWidget_ChartType,
  PerfChartSeries_PerfAggregationFunction,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

interface ChartWidgetToolbarProps {
  chartType: PerfChartWidget_ChartType;
  onChartTypeChange: (chartType: PerfChartWidget_ChartType) => void;
  currentGroupBy: string;
  onGroupByChange: (groupBy: string) => void;
  currentAggregation: PerfChartSeries_PerfAggregationFunction;
  onAggregationChange: (
    aggregation: PerfChartSeries_PerfAggregationFunction,
  ) => void;
}

export function ChartWidgetToolbar({
  chartType,
  onChartTypeChange,
  currentGroupBy,
  onGroupByChange,
  currentAggregation,
  onAggregationChange,
}: ChartWidgetToolbarProps) {
  const isDistribution =
    chartType === PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION;

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        pl: 2,
        pr: 0.5,
        py: 0.5,
        gap: 1,
      }}
    >
      <ToggleButtonGroup
        value={chartType}
        exclusive
        onChange={(_event, newChartType) => {
          if (newChartType !== null) {
            onChartTypeChange(newChartType);
          }
        }}
        size="small"
      >
        <ToggleButton
          value={PerfChartWidget_ChartType.MULTI_METRIC_CHART}
          title="Line Chart"
        >
          <ShowChartIcon fontSize="small" />
        </ToggleButton>
        <ToggleButton
          value={PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION}
          title="Scatter Plot"
        >
          <ScatterPlotIcon fontSize="small" />
        </ToggleButton>
      </ToggleButtonGroup>
      <Divider orientation="vertical" flexItem light />
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        <GroupWorkIcon sx={COMPACT_ICON_SX} />
        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
            fontWeight: (theme) => theme.typography.fontWeightBold,
            textTransform: 'uppercase',
            lineHeight: 1,
          }}
        >
          {COMMON_MESSAGES.GROUP_BY}
        </Typography>
      </Box>
      <FormControl size="small" variant="outlined">
        <Select
          id="widget-groupby-select"
          value={currentGroupBy}
          onChange={(e: SelectChangeEvent<string>) => {
            onGroupByChange(e.target.value);
          }}
          inputProps={{ 'aria-label': 'Widget Group By' }}
          sx={COMPACT_SELECT_SX}
        >
          {GROUP_BY_OPTIONS.map((opt) => (
            <MenuItem key={opt.value} value={opt.value}>
              {opt.label}
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      {!isDistribution && (
        <>
          <Divider orientation="vertical" flexItem light />

          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <FunctionsIcon sx={COMPACT_ICON_SX} />
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontWeight: (theme) => theme.typography.fontWeightBold,
                textTransform: 'uppercase',
                lineHeight: 1,
              }}
            >
              {COMMON_MESSAGES.AGGREGATE_BY}
            </Typography>
          </Box>
          <FormControl size="small" variant="outlined">
            <Select
              id="widget-aggregation-select"
              value={currentAggregation}
              onChange={(e) => {
                if (typeof e.target.value === 'number') {
                  onAggregationChange(e.target.value);
                }
              }}
              inputProps={{ 'aria-label': 'Widget Aggregation' }}
              sx={COMPACT_SELECT_SX}
            >
              {Object.entries(AGGREGATION_FUNCTION_LABELS)
                .filter(([value]) => Number(value) !== 0)
                .map(([value, label]) => (
                  <MenuItem key={value} value={Number(value)}>
                    {label}
                  </MenuItem>
                ))}
            </Select>
          </FormControl>
        </>
      )}
    </Box>
  );
}
