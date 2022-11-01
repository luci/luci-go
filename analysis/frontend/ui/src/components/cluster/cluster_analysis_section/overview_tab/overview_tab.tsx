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
} from 'react-router-dom';

import CircularProgress from '@mui/material/CircularProgress';
import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import TabPanel from '@mui/lab/TabPanel';
import Typography from '@mui/material/Typography';
import PanelHeading from '@/components/headings/panel_heading/panel_heading';

import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import ImpactTable from '@/components/cluster/cluster_analysis_section/overview_tab/impact_table/impact_table';
import useFetchCluster from '@/hooks/use_fetch_cluster';
import { ClusterContext } from '../../cluster_context';

interface Props {
  // The name of the tab.
  value: string;
}

const OverviewTab = ({
  value,
}: Props) => {
  const clusterId = useContext(ClusterContext);

  const {
    isLoading,
    isSuccess,
    data: cluster,
    error,
  } = useFetchCluster(clusterId.project, clusterId.algorithm, clusterId.id);
  return (
    <TabPanel value={value}>
      <PanelHeading>Impact</PanelHeading>
      {
        isLoading && (
          <Grid container item alignItems="center" justifyContent="center">
            <CircularProgress />
          </Grid>
        )
      }
      {
        error && (
          <LoadErrorAlert
            entityName="cluster impact"
            error={error}
          />
        )
      }
      {
        isSuccess && cluster && (
          <ImpactTable cluster={cluster}></ImpactTable>
        )
      }
      <Typography paddingTop='2rem'>To see examples of failures in this cluster, view <Link component={RouterLink} to='#recent-failures'>Recent Failures</Link>.</Typography>
    </TabPanel>
  );
};

export default OverviewTab;
