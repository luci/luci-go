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

import { useParams } from 'react-router-dom';

import Grid from '@mui/material/Grid';

import ClusterInfo from '@/components/cluster/cluster_info/cluster_info';
import ReclusteringProgressIndicator from '@/components/reclustering_progress_indicator/reclustering_progress_indicator';
import ErrorAlert from '@/components/error_alert/error_alert';

const ClusterTopPanel = () => {
  const { project } = useParams();
  if (!project) {
    return (
      <ErrorAlert
        errorTitle="Project is not specified"
        errorText="Project not specified in the URL, please make sure you have the correct URL and try again."
        showError/>
    );
  }
  return (
    <Grid container columnSpacing={2}>
      <Grid item xs={12}>
        <ReclusteringProgressIndicator
          hasRule={true}
          project={project}/>
      </Grid>
      <Grid item xs={12}>
        <ClusterInfo />
      </Grid>
    </Grid>
  );
};

export default ClusterTopPanel;
