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
import {
  Link as RouterLink,
  useSearchParams,
} from 'react-router-dom';

import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';

import { ClusterSummary } from '@/services/cluster';
import { linkToCluster } from '@/tools/urlHandling/links';

import { ClusterTableContextData } from '../clusters_table_context';

interface Props {
  project: string,
  cluster: ClusterSummary,
}

const ClustersTableRow = ({
  project,
  cluster,
}: Props) => {
  const [searchParams] = useSearchParams();
  const metrics = useContext(ClusterTableContextData).metrics || [];
  const selectedMetricsParam = searchParams.get('selectedMetrics') || '';

  const selectedMetrics = selectedMetricsParam?.split(',') || [];
  const filteredMetrics = metrics.filter((m) => selectedMetrics.indexOf(m.metricId) > -1);
  return (
    <TableRow>
      <TableCell data-testid="clusters_table_title">
        <Link component={RouterLink} to={linkToCluster(project, cluster.clusterId)} underline="hover">{cluster.title}</Link>
      </TableCell>
      <TableCell data-testid="clusters_table_bug">
        {
          cluster.bug &&
            <Link href={cluster.bug.url} underline="hover">{cluster.bug.linkText}</Link>
        }
      </TableCell>
      {
        filteredMetrics.map((metric) => {
          const metrics = cluster.metrics || {};
          const metricValue = metrics[metric.metricId] || { value: '' };
          return (
            <TableCell key={metric.metricId} className="number">{metricValue.value || '0'}</TableCell>
          );
        })
      }
    </TableRow>
  );
};

export default ClustersTableRow;
