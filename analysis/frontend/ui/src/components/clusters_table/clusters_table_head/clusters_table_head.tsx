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

import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TableSortLabel from '@mui/material/TableSortLabel';

import { ClusterTableContextData } from '@/components/clusters_table/clusters_table_context';
import {
  OrderBy,
  useOrderByParam,
  useSelectedMetricsParam,
} from '@/components/clusters_table/hooks';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';
import { MetricId } from '@/types/metric_id';

const ClustersTableHead = () => {
  const { metrics } = useContext(ClusterTableContextData);

  const [orderBy, updateOrderByParams] = useOrderByParam(metrics);

  const [selectedMetrics] = useSelectedMetricsParam(metrics);
  const filteredMetrics = metrics.filter((m) => selectedMetrics.indexOf(m) > -1);

  const handleOrderByChanged = (newOrderBy: OrderBy) => {
    updateOrderByParams(newOrderBy);
  };

  const toggleSort = (metric: MetricId) => {
    if (orderBy && orderBy.metric === metric) {
      handleOrderByChanged({
        metric: metric,
        isAscending: !orderBy.isAscending,
      });
    } else {
      handleOrderByChanged({
        metric: metric,
        isAscending: false,
      });
    }
  };

  return (
    <TableHead data-testid="clusters_table_head">
      <TableRow>
        <TableCell>Cluster</TableCell>
        <TableCell sx={{ width: '150px' }}>Bug</TableCell>
        {
          filteredMetrics.map((metric: ProjectMetric) => (
            <TableCell
              key={metric.metricId}
              sortDirection={(orderBy && (orderBy.metric === metric.metricId)) ? (orderBy.isAscending ? 'asc' : 'desc') : false}
              sx={{ cursor: 'pointer', width: '100px' }}>
              <TableSortLabel
                aria-label={`Sort by ${metric.humanReadableName}`}
                active={orderBy && (orderBy.metric === metric.metricId)}
                direction={(orderBy && orderBy.isAscending) ? 'asc' : 'desc'}
                onClick={() => toggleSort(metric.metricId)}>
                {metric.humanReadableName}
              </TableSortLabel>
            </TableCell>
          ))
        }
      </TableRow>
    </TableHead>
  );
};

export default ClustersTableHead;
