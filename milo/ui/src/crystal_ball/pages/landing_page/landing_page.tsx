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

import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

import { TimeSeriesChart } from '@/crystal_ball/components/time_series_chart/time_series_chart';

enum SamplePerformanceMetricsDataKeys {
  X_AXIS = 'time',
  Y_AXIS = 'samplePerformanceMetric',
}

type SamplePerformanceMetricDataPoint = {
  [K in SamplePerformanceMetricsDataKeys]: number;
};

/**
 * A simple landing page component.
 */
export function LandingPage() {
  // TODO: b/453052787 - Replace with call to crystalballperf.googleapis.com
  const sampleChartData: SamplePerformanceMetricDataPoint[] = [
    {
      time: new Date('2025-10-30T01:00:00Z').getTime(),
      samplePerformanceMetric: 50,
    },
    {
      time: new Date('2025-10-30T02:00:00Z').getTime(),
      samplePerformanceMetric: 65,
    },
  ];

  return (
    <Box sx={{ padding: 2 }}>
      <Typography variant="body1">
        This is the landing page for Crystal Ball Peformance Metrics
        Visualizations.
      </Typography>
      <TimeSeriesChart
        data={sampleChartData}
        lines={[
          {
            dataKey: SamplePerformanceMetricsDataKeys.Y_AXIS,
            stroke: '#1976d2',
            name: 'Sample Performance Metric',
          },
        ]}
        chartTitle="Sample Time Series Chart"
        xAxisDataKey={SamplePerformanceMetricsDataKeys.X_AXIS}
        yAxisLabel="Sample Metric Units"
      />
    </Box>
  );
}

/**
 * Component export for Landing Page.
 */
export function Component() {
  return <LandingPage />;
}
