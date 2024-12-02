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

import { FormControl, Grid2 as Grid, InputLabel } from '@mui/material';
import { useContext } from 'react';

import { ClusterTableContextData } from '@/clusters/components/clusters_table/context/clusters_table_context';
import { useSelectedMetricsParam } from '@/clusters/components/clusters_table/hooks';
import MetricsSelector from '@/clusters/components/metrics_selector/metrics_selector';
import { MetricId } from '@/clusters/types/metric_id';

const ClustersTableMetricSelection = () => {
  const metrics = useContext(ClusterTableContextData).metrics;
  const [selectedMetrics, updateSelectedMetricsParam] =
    useSelectedMetricsParam(metrics);

  function handleSelectedMetricsChanged(selectedMetricsIds: MetricId[]) {
    // Only update if at least one metric has been selected.
    if (selectedMetricsIds.length > 0) {
      updateSelectedMetricsParam(
        metrics.filter((m) => selectedMetricsIds.indexOf(m.metricId) > -1),
      );
    }
  }

  return (
    <Grid>
      <FormControl
        data-testid="metrics-selection"
        sx={{
          width: '100%',
        }}
      >
        <InputLabel id="metrics-selection-label">Metrics</InputLabel>
        <MetricsSelector
          labelId="metrics-selection-label"
          metrics={metrics}
          selectedMetrics={selectedMetrics.map((m) => m.metricId)}
          handleSelectedMetricsChanged={handleSelectedMetricsChanged}
        />
      </FormControl>
    </Grid>
  );
};

export default ClustersTableMetricSelection;
