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

import { useContext } from 'react';

import CircularProgress from '@mui/material/CircularProgress';
import Grid from '@mui/material/Grid';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';

import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import useFetchClusters from '@/hooks/use_fetch_clusters';

import { ClusterTableContextData } from '../clusters_table_context';
import ClustersTableHead from '../clusters_table_head/clusters_table_head';
import ClustersTableRow from '../clusters_table_row/clusters_table_row';
import { useFilterParam as useFailureFilterParam, useOrderByParam, useSelectedMetricsParam } from '../hooks';

interface Props {
  project: string,
}

const ClustersTableContent = ({
  project,
}: Props) => {
  const metrics = useContext(ClusterTableContextData).metrics;

  const [failureFilter] = useFailureFilterParam();
  const [orderBy] = useOrderByParam(metrics);
  const [selectedMetrics] = useSelectedMetricsParam(metrics);

  const fetchMetrics = metrics.filter((m) => selectedMetrics.indexOf(m) > -1);

  const {
    isLoading,
    isSuccess,
    data: clusters,
    error,
  } = useFetchClusters(project,
      failureFilter,
      orderBy,
      fetchMetrics);

  const rows = clusters?.clusterSummaries || [];

  if (isLoading && !isSuccess) {
    return (
      <Grid container item alignItems="center" justifyContent="center">
        <CircularProgress />
      </Grid>
    );
  }

  if (error) {
    return (
      <LoadErrorAlert
        entityName="clusters"
        error={error}
      />
    );
  }

  return (
    <>
      <Grid item xs={12}>
        <Table size="small" sx={{ overflowWrap: 'anywhere' }}>
          <ClustersTableHead />
          {
            isSuccess && (
              <TableBody data-testid='clusters_table_body'>
                {
                  rows.map((row) => (
                    <ClustersTableRow
                      key={`${row.clusterId.algorithm}:${row.clusterId.id}`}
                      project={project}
                      cluster={row}
                    />
                  ))
                }
              </TableBody>
            )
          }
        </Table>
        {isSuccess && rows.length === 0 && (
          <Grid container item alignItems="center" justifyContent="center">
              Hooray! There are no failures matching the specified criteria.
          </Grid>
        )}
      </Grid>
    </>
  );
};

export default ClustersTableContent;
