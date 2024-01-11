// Copyright 2023 The LUCI Authors.
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
  Bar,
  BarChart,
  LabelList,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import { Metric } from '@/legacy_services/metrics';
import { ClusterHistoryDay } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

interface Props {
  height: number;
  color: string,
  isAnnotated: boolean,
  metric: Metric,
  data: readonly ClusterHistoryDay[],
}

export const SingleMetricChart = (
    { height, color, isAnnotated, metric, data }: Props) => {
  return (
    <ResponsiveContainer
      width="100%"
      height={height} >
      <BarChart
        data={data as ClusterHistoryDay[]} // Trust that barchart will not modify readonly data.
        syncId="impactMetrics"
        margin={{ top: 20, bottom: 20 }} >
        <XAxis dataKey="date" />
        <YAxis />
        <Legend />
        <Tooltip />
        <Bar
          name={metric.humanReadableName}
          dataKey={`metrics.${metric.metricId}`}
          fill={color}>
          {isAnnotated && (
            <LabelList dataKey={`metrics.${metric.metricId}`} position="top" />
          )}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  );
};
