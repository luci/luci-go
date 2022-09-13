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

import {
  Link,
  useParams,
} from 'react-router-dom';

import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';

import CodeBlock from '@/components/codeblock/codeblock';
import ErrorAlert from '@/components/error_alert/error_alert';
import useFetchCluster from '@/hooks/use_fetch_cluster';
import { Cluster } from '@/services/cluster';

interface ClusterDetailsProps {
  project: string;
  cluster: Cluster;
  criteriaName: string;
  clusterAlgorithm: string;
  clusterId: string;
}

const ClusterDetails = ({
  project,
  cluster,
  criteriaName,
  clusterAlgorithm,
  clusterId,
}: ClusterDetailsProps) => {
  const projectEncoded = encodeURIComponent(project);
  const ruleEncoded = encodeURIComponent(cluster.equivalentFailureAssociationRule || '');
  const sourceAlgEncoded = encodeURIComponent(clusterAlgorithm);
  const sourceIdEncoded = encodeURIComponent(clusterId);

  const newRuleURL = `/p/${projectEncoded}/rules/new?rule=${ruleEncoded}&sourceAlg=${sourceAlgEncoded}&sourceId=${sourceIdEncoded}`;

  return (
    <>
      <Typography sx={{
        fontWeight: 600,
        fontSize: 20,
        mb: 2,
      }}>
        {criteriaName}
      </Typography>
      <Grid
        container
        item
        alignItems="center"
        sx={{
          mb: 2,
        }}>
        <Box data-testid="cluster-definition" sx={{ display: 'grid' }}>
          <CodeBlock code={cluster.title} />
        </Box>
      </Grid>
      <Grid item xs={12}>
        <Button
          component={Link}
          variant='contained'
          to={newRuleURL}>
            create rule from cluster
        </Button>
      </Grid>
    </>
  );
};

const ClusterInfo = () => {
  const { project, algorithm, id } = useParams();

  const {
    isLoading,
    isError,
    isSuccess,
    data: cluster,
    error,
  } = useFetchCluster(project, algorithm, id);

  if (!algorithm) {
    return (
      <ErrorAlert
        errorTitle="Clustering algorithm not specified"
        errorText="Clustering algorithm was not found in the URL, please make sure you have the corrent URL format for the cluster."
        showError/>
    );
  }

  let criteriaName = '';
  if (algorithm.startsWith('testname-')) {
    criteriaName = 'Test name cluster';
  } else if (algorithm.startsWith('reason-')) {
    criteriaName = 'Failure reason cluster';
  }

  if (!project) {
    return (
      <ErrorAlert
        errorTitle="Project not specified"
        errorText="A project is required to load the cluster data, please check the URL and try again."
        showError/>
    );
  }

  if (!id) {
    return (
      <ErrorAlert
        errorTitle="ClusterID not specified"
        errorText="A cluster id is required to load the cluster data, please check the URL and try again."
        showError/>
    );
  }

  return (
    <Paper data-cy="cluster-info" elevation={3} sx={{ pt: 2, pb: 2, mt: 1 }} >
      <Container maxWidth={false}>
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
            <ClusterDetails
              project={project}
              cluster={cluster}
              criteriaName={criteriaName}
              clusterAlgorithm={algorithm}
              clusterId={id}
            />
          )
        }
      </Container>
    </Paper>
  );
};

export default ClusterInfo;
