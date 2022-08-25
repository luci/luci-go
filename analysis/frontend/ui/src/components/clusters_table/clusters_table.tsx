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

import {
  useState,
} from 'react';
import { useQuery } from 'react-query';
import { useSearchParams } from 'react-router-dom';

import CircularProgress from '@mui/material/CircularProgress';
import Grid from '@mui/material/Grid';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';

import { getClustersService, QueryClusterSummariesRequest, SortableMetricName } from '../../services/cluster';

import ErrorAlert from '../error_alert/error_alert';
import ClustersTableFilter from './clusters_table_filter/clusters_table_filter';
import ClustersTableRow from './clusters_table_row/clusters_table_row';
import ClustersTableHead from './clusters_table_head/clusters_table_head';

interface Props {
    project: string;
}

const ClustersTable = ({
  project,
}: Props) => {
  const [searchParams, setSearchParams] = useSearchParams();

  const [sortMetric, setCurrentSortMetric] = useState<SortableMetricName>('critical_failures_exonerated');
  const [isAscending, setIsAscending] = useState(false);

  const clustersService = getClustersService();

  const {
    isLoading,
    isError,
    isSuccess,
    data: clusters,
    error,
  } = useQuery(
      ['clusters', project, searchParams.get('q') || '', sortMetric, isAscending ? 'asc' : 'desc'],
      async () => {
        const request : QueryClusterSummariesRequest = {
          project: project,
          failureFilter: searchParams.get('q') || '',
          orderBy: sortMetric + (isAscending ? '' : ' desc'),
        };

        return await clustersService.queryClusterSummaries(request);
      },
  );


  const onFailureFilterChanged = (filter: string) => {
    setSearchParams({ 'q': filter }, { replace: true });
  };

  const toggleSort = (metric: SortableMetricName) => {
    if (metric === sortMetric) {
      setIsAscending(!isAscending);
    } else {
      setCurrentSortMetric(metric);
      setIsAscending(false);
    }
  };

  const rows = clusters?.clusterSummaries || [];

  return (
    <Grid container columnGap={2} rowGap={2}>
      <ClustersTableFilter
        failureFilter={searchParams.get('q') || ''}
        setFailureFilter={onFailureFilterChanged}/>
      <Grid item xs={12}>
        <Table size="small" sx={{ overflowWrap: 'anywhere' }}>
          <ClustersTableHead
            toggleSort={toggleSort}
            sortMetric={sortMetric}
            isAscending={isAscending}/>
          {
            isSuccess && (
              <TableBody data-testid='clusters_table_body'>
                {
                  rows.map((row) => (
                    <ClustersTableRow
                      key={`${row.clusterId.algorithm}:${row.clusterId.id}`}
                      project={project}
                      cluster={row}/>
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
        isError && (
          <ErrorAlert
            errorTitle="Failed to load failures"
            errorText={`Loading cluster failures failed due to: ${error}`}
            showError/>
        )
      }
      {
        isLoading && (
          <Grid container item alignItems="center" justifyContent="center">
            <CircularProgress />
          </Grid>
        )
      }
    </Grid>
  );
};

export default ClustersTable;
