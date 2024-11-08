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

import Grid from '@mui/material/Grid';

import ClustersTableFilter from './clusters_table_filter/clusters_table_filter';
import { ClustersTableIntervalSelection } from './clusters_table_interval_selection/clusters_table_interval_selection';
import ClustersTableMetricSelection from './clusters_table_metric_selection/clusters_table_metric_selection';

const ClustersTableForm = () => {
  return (
    <Grid
      container
      item
      xs={12}
      spacing={2}
      data-testid="clusters_table_filter"
    >
      <Grid item xs={6}>
        <ClustersTableFilter />
      </Grid>
      <Grid item xs={2}>
        <ClustersTableIntervalSelection />
      </Grid>
      <Grid item xs={4}>
        <ClustersTableMetricSelection />
      </Grid>
    </Grid>
  );
};

export default ClustersTableForm;
