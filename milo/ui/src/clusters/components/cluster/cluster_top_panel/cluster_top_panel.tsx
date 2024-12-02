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

import Grid from '@mui/material/Grid2';
import { useContext } from 'react';

import ClusterInfo from '@/clusters/components/cluster/cluster_info/cluster_info';
import ReclusteringProgressIndicator from '@/clusters/components/reclustering_progress_indicator/reclustering_progress_indicator';

import { ClusterContext } from '../cluster_context';

const ClusterTopPanel = () => {
  const { project } = useContext(ClusterContext);

  return (
    <Grid container columnSpacing={2}>
      <Grid size={12}>
        <ReclusteringProgressIndicator hasRule={true} project={project} />
      </Grid>
      <Grid size={12}>
        <ClusterInfo />
      </Grid>
    </Grid>
  );
};

export default ClusterTopPanel;
