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

import { useQuery } from 'react-query';
import { useParams } from 'react-router-dom';

import Container from '@mui/material/Container';
import CircularProgress from '@mui/material/CircularProgress';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';

import { getClustersService, BatchGetClustersRequest } from '../../services/cluster';
import ErrorAlert from '../error_alert/error_alert';
import ImpactTable from '../impact_table/impact_table';

const ImpactSection = () => {
  const { project, algorithm, id } = useParams();
  let currentAlgorithm = algorithm;
  if (!currentAlgorithm) {
    currentAlgorithm = 'rules';
  }
  const clustersService = getClustersService();
  const { isLoading, isError, isSuccess, data: cluster, error } = useQuery(['cluster', project, currentAlgorithm, id], async () => {
    const request: BatchGetClustersRequest = {
      parent: `projects/${encodeURIComponent(project || '')}`,
      names: [
        `projects/${encodeURIComponent(project || '')}/clusters/${encodeURIComponent(currentAlgorithm || '')}/${encodeURIComponent(id || '')}`,
      ],
    };

    const response = await clustersService.batchGet(request);

    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    return response.clusters![0];
  });

  return (
    <Paper elevation={3} sx={{ pt: 1, pb: 4 }}>
      <Container maxWidth={false}>
        <h2>Impact</h2>
        {
          isLoading && (
            <Grid container item alignItems="center" justifyContent="center">
              <CircularProgress />
            </Grid>
          )
        }
        {
          isError && (
            <ErrorAlert
              errorText={`Got an error while loading the cluster: ${error}`}
              errorTitle="Failed to load cluster"
              showError/>
          )
        }
        {
          isSuccess && cluster && (
            <ImpactTable cluster={cluster}></ImpactTable>
          )
        }
      </Container>
    </Paper>
  );
};

export default ImpactSection;
