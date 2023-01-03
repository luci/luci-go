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

import CircularProgress from '@mui/material/CircularProgress';
import Grid from '@mui/material/Grid';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';

import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import useFetchClusters from '@/hooks/use_fetch_clusters';

import {
  Metric,
} from '@/services/metrics';

import ClustersTableHead, {
  OrderBy,
} from '../clusters_table_head/clusters_table_head';
import ClustersTableRow from '../clusters_table_row/clusters_table_row';

interface Props {
  project: string,
  failureFilter: string,
  orderBy?: OrderBy,
  metrics: Metric[],
  handleOrderByChanged: (orderBy: OrderBy) => void,
}

const ClustersTableContent = ({
  project,
  failureFilter,
  orderBy,
  metrics,
  handleOrderByChanged,
}: Props) => {
  const {
    isLoading,
    isSuccess,
    data: clusters,
    error,
  } = useFetchClusters(project, failureFilter, orderBy, metrics);

  const rows = clusters?.clusterSummaries || [];

  return (
    <>
      <Grid item xs={12}>
        <Table size="small" sx={{ overflowWrap: 'anywhere' }}>
          <ClustersTableHead
            handleOrderByChanged={handleOrderByChanged}
            orderBy={orderBy}
            metrics={metrics}/>
          {
            isSuccess && (
              <TableBody data-testid='clusters_table_body'>
                {
                  rows.map((row) => (
                    <ClustersTableRow
                      key={`${row.clusterId.algorithm}:${row.clusterId.id}`}
                      project={project}
                      cluster={row}
                      metrics={metrics}/>
                  ))
                }
              </TableBody>
            )
          }
        </Table>
      </Grid>
      {
        isSuccess && rows.length === 0 && (
          <Grid container item alignItems="center" justifyContent="center">
            Hooray! There are no failures matching the specified criteria.
          </Grid>
        )
      }
      {
        error && (
          <LoadErrorAlert
            entityName="clusters"
            error={error}
          />
        )
      }
      {
        isLoading && (
          <Grid container item alignItems="center" justifyContent="center">
            <CircularProgress />
          </Grid>
        )
      }
    </>
  );
};

export default ClustersTableContent;
