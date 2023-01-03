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

import { useSearchParams, ParamKeyValuePair } from 'react-router-dom';

import CircularProgress from '@mui/material/CircularProgress';
import Grid from '@mui/material/Grid';

import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import useFetchMetrics from '@/hooks/use_fetch_metrics';
import { Metric } from '@/services/metrics';

import ClustersTableFilter from './clusters_table_filter/clusters_table_filter';
import { OrderBy } from './clusters_table_head/clusters_table_head';
import ClustersTableContent from './clusters_table_content/clusters_table_content';

interface Props {
    project: string;
}

const ClustersTable = ({
  project,
}: Props) => {
  const [searchParams, setSearchParams] = useSearchParams();

  const {
    isLoading,
    isSuccess,
    data: allMetrics,
    error,
  } = useFetchMetrics();

  const metrics = allMetrics?.filter((m) => m.isDefault) || [];

  let orderBy: OrderBy | undefined;
  const orderByMetricId = searchParams.get('orderBy') || '';
  if (orderByMetricId) {
    // Ensure the metric we are being asked to order by
    // is one of the metrics we are querying.
    if (metrics.some((m) => m.metricId == orderByMetricId)) {
      orderBy = {
        metric: orderByMetricId,
        isAscending: searchParams.get('orderDir') === 'asc',
      };
    }
  }

  // Prepare the default order by, but do not assign it
  // to orderBy to avoid it becoming part of the URL
  // on a query change. Only 'order by's explicitly selected
  // by the user should form part of the URL.
  let defaultOrderBy: OrderBy | undefined;
  let maxSortPriorityMetric: Metric | undefined;
  for (let i = 0; i < metrics.length; i++) {
    if (maxSortPriorityMetric === undefined || metrics[i].sortPriority > maxSortPriorityMetric.sortPriority) {
      maxSortPriorityMetric = metrics[i];
    }
  }
  if (maxSortPriorityMetric) {
    defaultOrderBy = {
      metric: maxSortPriorityMetric.metricId,
      isAscending: false,
    };
  }

  const failureFilter = searchParams.get('q') || '';

  const handleOrderByChanged = (newOrderBy: OrderBy) => {
    updateSearchParams(failureFilter, newOrderBy);
  };

  const handleFailureFilterChanged = (newFilter: string) => {
    if (newFilter == failureFilter) {
      return;
    }
    updateSearchParams(newFilter, orderBy);
  };

  const updateSearchParams = (failureFilter: string, orderBy?: OrderBy) => {
    const params : ParamKeyValuePair[] = [];
    if (failureFilter !== '') {
      params.push(['q', failureFilter]);
    }
    if (orderBy) {
      params.push(['orderBy', orderBy.metric]);
      if (orderBy.isAscending) {
        params.push(['orderDir', 'asc']);
      }
    }
    setSearchParams(params);
  };

  return (
    <Grid container columnGap={2} rowGap={2}>
      <ClustersTableFilter
        failureFilter={failureFilter}
        handleFailureFilterChanged={handleFailureFilterChanged}/>
      {
        error && (
          <LoadErrorAlert
            entityName="metrics"
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
      {
        isSuccess && metrics !== undefined && (
          <ClustersTableContent
            project={project}
            failureFilter={failureFilter}
            orderBy={orderBy || defaultOrderBy}
            metrics={metrics}
            handleOrderByChanged={handleOrderByChanged} />
        )
      }
    </Grid>
  );
};

export default ClustersTable;
