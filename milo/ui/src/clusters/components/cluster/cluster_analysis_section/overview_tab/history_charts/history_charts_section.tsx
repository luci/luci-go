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

import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';
import { useContext, useEffect } from 'react';
import { Link as RouterLink } from 'react-router-dom';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { ClusterContext } from '@/clusters/components/cluster/cluster_context';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import useQueryClusterHistory from '@/clusters/hooks/use_query_cluster_history';
import { getMetricColor } from '@/clusters/tools/metric_colors';

import { OverviewTabContextData } from '../overview_tab_context';

import { HISTORY_TIME_RANGE_OPTIONS } from './history_charts_form/constants';
import { HistoryChartsForm } from './history_charts_form/history_charts_form';
import {
  useAnnotatedParam,
  useHistoryTimeRangeParam,
  useSelectedMetricsParam,
} from './hooks';
import { SingleMetricChart } from './single_metric_chart';

export const HistoryChartsSection = () => {
  const clusterId = useContext(ClusterContext);
  const { metrics } = useContext(OverviewTabContextData);

  const [isAnnotated, updateAnnotatedParam] = useAnnotatedParam();
  const [selectedHistoryTimeRange, updateHistoryTimeRangeParam] =
    useHistoryTimeRangeParam(HISTORY_TIME_RANGE_OPTIONS);
  const [selectedMetrics, updateSelectedMetricsParam] =
    useSelectedMetricsParam(metrics);

  // Set annotations off by default.
  useEffect(() => {
    if (isAnnotated === undefined) {
      updateAnnotatedParam(false, true);
    }
  }, [isAnnotated, updateAnnotatedParam]);

  // Set the default day range if there isn't one in the URL already.
  useEffect(() => {
    if (!selectedHistoryTimeRange) {
      updateHistoryTimeRangeParam(HISTORY_TIME_RANGE_OPTIONS[0], true);
    }
  }, [selectedHistoryTimeRange, updateHistoryTimeRangeParam]);

  // Set the default selected metrics if there are none in the URL already.
  useEffect(() => {
    if (!selectedMetrics.length && metrics) {
      const defaultMetrics = metrics?.filter((m) => m.isDefault);
      updateSelectedMetricsParam(defaultMetrics, true);
    }
  }, [metrics, selectedMetrics, updateSelectedMetricsParam]);

  const days = selectedHistoryTimeRange?.value || 0;

  // Note that querying the history of a single cluster is faster and cheaper.
  const { isLoading, isSuccess, data, error } = useQueryClusterHistory(
    clusterId.project,
    `cluster_algorithm="${clusterId.algorithm}" cluster_id="${clusterId.id}"`,
    days,
    selectedMetrics.map((m) => m.name),
  );

  // Calculate each chart's column size, given that a chart with data for the
  // maximum of 90 days should use a full row.
  const rows = Math.ceil((selectedMetrics.length * days) / 90);
  let itemSize = 30;
  if (selectedMetrics.length > 0) {
    itemSize = Math.floor((rows * 90) / selectedMetrics.length);
  }

  // Reduce chart height if all charts don't fit in 1 row.
  const chartHeight = rows > 1 ? 200 : 400;

  return (
    <div>
      <HistoryChartsForm />
      {error && <LoadErrorAlert entityName="cluster history" error={error} />}
      {!error && isLoading && <CentralizedProgress />}
      {isSuccess && data && metrics && selectedMetrics.length > 0 ? (
        <Grid container columns={90} data-testid="history-charts-container">
          {selectedMetrics.map((m) => (
            <Grid
              item
              key={m.metricId}
              xs={itemSize}
              data-testid={'chart-' + m.metricId}
            >
              <SingleMetricChart
                height={chartHeight}
                color={getMetricColor(metrics.indexOf(m))}
                isAnnotated={isAnnotated || false}
                metric={m}
                data={data.days}
              />
            </Grid>
          ))}
        </Grid>
      ) : (
        <Typography color="GrayText">
          Select some metrics to see its history.
        </Typography>
      )}
      <div style={{ paddingTop: '2rem' }}>
        {selectedMetrics.length > 0 && (
          <Typography>
            This chart shows the history of metrics for this cluster for each
            day in the selected time period.
          </Typography>
        )}
        <Typography>
          To see examples of failures in this cluster, view{' '}
          <Link component={RouterLink} to="#recent-failures">
            Recent Failures
          </Link>
          .
        </Typography>
      </div>
    </div>
  );
};
