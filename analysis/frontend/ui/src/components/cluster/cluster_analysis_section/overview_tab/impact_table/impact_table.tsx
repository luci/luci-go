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

import Box from '@mui/material/Box';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import HelpTooltip from '@/components/help_tooltip/help_tooltip';
import {
  Cluster,
  ClusterMetrics,
  Counts,
} from '@/services/cluster';
import {
  Metric,
} from '@/services/metrics';

interface Props {
    cluster: Cluster;
    metrics: Metric[];
}

const ImpactTable = ({ cluster, metrics }: Props) => {
  const metric = (counts: Counts): string => {
    return counts.nominal || '0';
  };

  let clusterMetrics: ClusterMetrics = {};
  if (cluster.metrics !== undefined) {
    clusterMetrics = cluster.metrics;
  }

  return (
    <TableContainer component={Box}>
      <Table data-testid="impact-table" size="small" sx={{ maxWidth: 600 }}>
        <TableHead>
          <TableRow>
            <TableCell></TableCell>
            <TableCell align="right">1 day</TableCell>
            <TableCell align="right">3 days</TableCell>
            <TableCell align="right">7 days</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {
            metrics.filter((m) => m.isDefault && m.metricId in clusterMetrics).map((m) => {
              const timewiseCounts = clusterMetrics[m.metricId];
              return (
                <TableRow key={m.metricId}>
                  <TableCell>{m.humanReadableName} <HelpTooltip text={m.description} /></TableCell>
                  <TableCell align="right">{metric(timewiseCounts.oneDay)}</TableCell>
                  <TableCell align="right">{metric(timewiseCounts.threeDay)}</TableCell>
                  <TableCell align="right">{metric(timewiseCounts.sevenDay)}</TableCell>
                </TableRow>
              );
            })
          }
        </TableBody>
      </Table>
    </TableContainer>
  );
};

export default ImpactTable;
