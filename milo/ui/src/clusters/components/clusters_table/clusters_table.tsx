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

import Grid from '@mui/material/Grid';
import { useEffect } from 'react';

import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import useFetchMetrics from '@/clusters/hooks/use_fetch_metrics';

import CentralizedProgress from '../centralized_progress/centralized_progress';

import ClustersTableContent from './clusters_table_content/clusters_table_content';
import ClustersTableForm from './clusters_table_form/clusters_table_form';
import { TIME_INTERVAL_OPTIONS } from './clusters_table_form/clusters_table_interval_selection/constants';
import { ClusterTableContextProvider } from './context';
import { useIntervalParam, useSelectedMetricsParam } from './hooks';

interface Props {
  project: string;
}

const ClustersTable = ({ project }: Props) => {
  const {
    isLoading,
    isSuccess,
    data: metrics,
    error,
  } = useFetchMetrics(project);

  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const [selectedMetrics, updateSelectedMetricsParam] = useSelectedMetricsParam(
    metrics || [],
  );

  // Set the default order by and the selected metrics
  // if there are none in the URL already.
  useEffect(() => {
    if (!selectedMetrics.length && metrics) {
      const defaultMetrics = metrics?.filter((m) => m.isDefault);
      updateSelectedMetricsParam(defaultMetrics, true);
    }
  }, [metrics, selectedMetrics, updateSelectedMetricsParam]);

  const [selectedInterval, updateIntervalParam] = useIntervalParam(
    TIME_INTERVAL_OPTIONS,
  );

  // Set the default selected interval to be the first interval option
  // if there are none in the URL already.
  useEffect(() => {
    if (!selectedInterval) {
      updateIntervalParam(TIME_INTERVAL_OPTIONS[0], true);
    }
  }, [selectedInterval, updateIntervalParam]);

  return (
    <ClusterTableContextProvider metrics={metrics}>
      <Grid container columnGap={2} rowGap={2}>
        <ClustersTableForm />
        {error && <LoadErrorAlert entityName="metrics" error={error} />}
        {isLoading && <CentralizedProgress />}
        {isSuccess && metrics !== undefined && (
          <ClustersTableContent project={project} />
        )}
      </Grid>
    </ClusterTableContextProvider>
  );
};

export default ClustersTable;
