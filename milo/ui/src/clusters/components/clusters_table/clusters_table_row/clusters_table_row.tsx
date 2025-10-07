// Copyright 2022 The LUCI Authors.
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
import Link from '@mui/material/Link';
import Skeleton from '@mui/material/Skeleton';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import { useContext, useMemo } from 'react';
import { Link as RouterLink } from 'react-router';

import { getMetricColor } from '@/clusters/tools/metric_colors';
import { linkToCluster } from '@/clusters/tools/urlHandling/links';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ClusterSummary } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';

import { ClusterTableContextData } from '../context/clusters_table_context';

interface SparklineProps {
  isLoading?: boolean;
  isSuccess?: boolean;
  values: readonly string[];
  color: string;
}

const Sparkline = ({ isLoading, isSuccess, values, color }: SparklineProps) => {
  // The dimensions of the sparkline element, in pixels.
  const sparklineWidth = 100;
  const sparklineHeight = 30;

  if (isLoading && !isSuccess) {
    return (
      <Skeleton
        data-testid="clusters_table_sparkline_skeleton"
        variant="rectangular"
        width={sparklineWidth}
        height={sparklineHeight}
      />
    );
  }

  if (!values || values.length < 2) {
    return null;
  }

  // Note the maximum height is set to be 5% more than the actual maximum value,
  // so the top of plots still have space for the thick day-to-day polyline.
  const parsedValues = values.map((v) => parseInt(v, 10));
  const max = 1.05 * Math.max(...parsedValues) || 1;

  const daySegment = sparklineWidth / values.length;
  const barWidth = daySegment * 0.7;
  const barOffset = (daySegment - barWidth) / 2;

  const coords: string[] = [];
  parsedValues.forEach((value, i) => {
    const x = sparklineWidth - (i + 0.5) * daySegment;
    const y = sparklineHeight - (sparklineHeight * value) / max;
    coords.push(`${x},${y}`);
  });

  return (
    <Box sx={{ width: sparklineWidth, height: sparklineHeight }}>
      <svg
        data-testid="clusters_table_sparkline"
        viewBox={`0 0 ${sparklineWidth} ${sparklineHeight}`}
        xmlns="http://www.w3.org/2000/svg"
        style={{ width: '100%', height: 'auto' }}
      >
        <line
          x1={0}
          y1={sparklineHeight}
          x2={sparklineWidth}
          y2={sparklineHeight}
          stroke={color}
        />
        {parsedValues.map((value, i) => {
          const x = sparklineWidth - (i + 1) * daySegment + barOffset;
          const height = (sparklineHeight * value) / max;
          return (
            <rect
              key={i}
              x={x}
              y={sparklineHeight - height}
              width={barWidth}
              height={height}
              fill={color}
              fillOpacity="10%"
            />
          );
        })}
        <polyline
          points={coords.join(' ')}
          fill="none"
          strokeWidth={2}
          stroke={color}
        />
      </svg>
    </Box>
  );
};

interface Props {
  project: string;
  cluster: ClusterSummary;
  isBreakdownLoading?: boolean;
  isBreakdownSuccess?: boolean;
}

const ClustersTableRow = ({
  project,
  cluster,
  isBreakdownLoading,
  isBreakdownSuccess,
}: Props) => {
  const [searchParams] = useSyncedSearchParams();
  const metrics = useContext(ClusterTableContextData).metrics || [];
  const selectedMetricsParam = searchParams.get('selectedMetrics') || '';

  const selectedMetrics = selectedMetricsParam?.split(',') || [];
  const filteredMetrics = metrics.filter(
    (m) => selectedMetrics.indexOf(m.metricId) > -1,
  );
  return (
    <TableRow>
      <TableCell data-testid="clusters_table_title">
        <Link
          component={RouterLink}
          to={linkToCluster(project, cluster.clusterId!)}
          underline="hover"
        >
          {cluster.title}
        </Link>
      </TableCell>
      <TableCell data-testid="clusters_table_bug">
        {cluster.bug && (
          <Link href={cluster.bug.url} underline="hover">
            {cluster.bug.linkText}
          </Link>
        )}
      </TableCell>
      {filteredMetrics.map((metric) => (
        <ClusterRowMetrics
          key={metric.metricId}
          cluster={cluster}
          metric={metric}
          metrics={metrics}
          isBreakdownLoading={isBreakdownLoading}
          isBreakdownSuccess={isBreakdownSuccess}
        />
      ))}
    </TableRow>
  );
};

interface ClusterRowMetricsProps {
  cluster: ClusterSummary;
  metric: ProjectMetric;
  metrics: ProjectMetric[];
  isBreakdownSuccess: boolean | undefined;
  isBreakdownLoading: boolean | undefined;
}

function ClusterRowMetrics({
  cluster,
  metric,
  metrics,
  isBreakdownSuccess,
  isBreakdownLoading,
}: ClusterRowMetricsProps) {
  const clusterMetrics = cluster.metrics || {};
  const metricValue = clusterMetrics[metric.metricId] || {
    value: '',
    dailyBreakdown: [],
  };

  const metricColor = getMetricColor(metrics.indexOf(metric));
  const dailyBreakdown = useMemo(
    () => metricValue.dailyBreakdown || [],
    [metricValue.dailyBreakdown],
  );

  // A sparkline may be added if the daily breakdown for the metric is:
  // 1. being fetched; or
  // 2. fetched and there are at least 2 data points.
  const hasSparkline = useMemo(() => {
    return (
      (isBreakdownLoading && !isBreakdownSuccess) ||
      (isBreakdownSuccess && dailyBreakdown && dailyBreakdown.length > 1)
    );
  }, [dailyBreakdown, isBreakdownLoading, isBreakdownSuccess]);
  return (
    <TableCell key={metric.metricId} className="number">
      {metricValue.value || '0'}
      {hasSparkline && (
        <Sparkline
          isLoading={isBreakdownLoading}
          isSuccess={isBreakdownSuccess}
          values={dailyBreakdown}
          color={metricColor}
        />
      )}
    </TableCell>
  );
}

export default ClustersTableRow;
