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

import CircularProgress from '@mui/material/CircularProgress';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';

import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import ImpactTable from '@/components/impact_table/impact_table';
import useFetchCluster from '@/hooks/use_fetch_cluster';

const ImpactSection = () => {
  const { project, algorithm, id } = useParams();
  let currentAlgorithm = algorithm;
  if (!currentAlgorithm) {
    currentAlgorithm = 'rules';
  }
  const {
    isLoading,
    isSuccess,
    data: cluster,
    error,
  } = useFetchCluster(project, currentAlgorithm, id);
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
      </Container>
    </Paper>
  );
};

export default ImpactSection;
