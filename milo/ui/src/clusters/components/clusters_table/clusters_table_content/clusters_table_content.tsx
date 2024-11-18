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
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import { useContext } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import {
  ClustersFetchOptions,
  useFetchClusterSummaries,
} from '@/clusters/hooks/use_fetch_clusters';
import { ClusterSummaryView } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

import { TIME_INTERVAL_OPTIONS } from '../clusters_table_form/clusters_table_interval_selection/constants';
import ClustersTableHead from '../clusters_table_head/clusters_table_head';
import ClustersTableRow from '../clusters_table_row/clusters_table_row';
import { ClusterTableContextData } from '../context/clusters_table_context';
import {
  useFilterParam as useFailureFilterParam,
  useIntervalParam,
  useOrderByParam,
  useSelectedMetricsParam,
} from '../hooks';

interface Props {
  project: string;
}

const ClustersTableContent = ({ project }: Props) => {
  const metrics = useContext(ClusterTableContextData).metrics;

  const [failureFilter] = useFailureFilterParam();
  const [interval] = useIntervalParam(TIME_INTERVAL_OPTIONS);
  const [orderBy] = useOrderByParam(metrics);
  const [selectedMetrics] = useSelectedMetricsParam(metrics);

  const fetchMetrics = metrics.filter((m) => selectedMetrics.indexOf(m) > -1);

  const fetchOptions: ClustersFetchOptions = {
    project: project,
    failureFilter: failureFilter,
    orderBy: orderBy,
    metrics: fetchMetrics,
    interval: interval,
  };
  const {
    isLoading: isBasicSummariesLoading,
    isSuccess: isBasicSummariesSuccess,
    data: basicSummaries,
    error: basicSummariesError,
  } = useFetchClusterSummaries(fetchOptions, ClusterSummaryView.BASIC);
  const {
    isInitialLoading: isFullSummariesLoading,
    isSuccess: isFullSummariesSuccess,
    data: fullSummaries,
  } = useFetchClusterSummaries(fetchOptions, ClusterSummaryView.FULL);
  // Note: it is expected that the call for basic cluster summaries will return
  // before the call for full cluster summaries. Full cluster summaries will be
  // displayed when available, and will replace basic cluster summaries.
  const rows =
    fullSummaries?.clusterSummaries || basicSummaries?.clusterSummaries || [];

  if (isBasicSummariesLoading && !isBasicSummariesSuccess) {
    return <CentralizedProgress />;
  }

  if (basicSummariesError) {
    return <LoadErrorAlert entityName="clusters" error={basicSummariesError} />;
  }

  return (
    <Grid item xs={12}>
      <Table size="small" sx={{ overflowWrap: 'anywhere' }}>
        <ClustersTableHead />
        {isBasicSummariesSuccess && (
          <TableBody data-testid="clusters_table_body">
            {rows.map((row) => {
              return (
                <ClustersTableRow
                  // Cluster ID will be set on all clusters.
                  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
                  key={`${row.clusterId!.algorithm}:${row.clusterId!.id}`}
                  project={project}
                  cluster={row}
                  isBreakdownLoading={isFullSummariesLoading}
                  isBreakdownSuccess={isFullSummariesSuccess}
                />
              );
            })}
          </TableBody>
        )}
      </Table>
      {isBasicSummariesSuccess && rows.length === 0 && (
        <Grid container item alignItems="center" justifyContent="center">
          Hooray! There are no failures matching the specified criteria.
        </Grid>
      )}
    </Grid>
  );
};

export default ClustersTableContent;
