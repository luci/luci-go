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
import Button from '@mui/material/Button';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid2';
import Paper from '@mui/material/Paper';
import { useContext } from 'react';
import { Link } from 'react-router-dom';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import CodeBlock from '@/clusters/components/codeblock/codeblock';
import PanelHeading from '@/clusters/components/headings/panel_heading/panel_heading';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import useFetchCluster from '@/clusters/hooks/use_fetch_cluster';
import { Cluster } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

import { ClusterContext } from '../cluster_context';

interface ClusterDetailsProps {
  cluster: Cluster;
}

const ClusterDetails = ({ cluster }: ClusterDetailsProps) => {
  const {
    project,
    algorithm: clusterAlgorithm,
    id: clusterId,
  } = useContext(ClusterContext);

  const projectEncoded = encodeURIComponent(project);
  const ruleEncoded = encodeURIComponent(
    cluster.equivalentFailureAssociationRule || '',
  );
  const sourceAlgEncoded = encodeURIComponent(clusterAlgorithm);
  const sourceIdEncoded = encodeURIComponent(clusterId);

  const newRuleURL =
    `/ui/tests/p/${projectEncoded}` +
    `/rules/new?rule=${ruleEncoded}&sourceAlg=${sourceAlgEncoded}&sourceId=${sourceIdEncoded}`;

  return (
    <>
      <Grid
        container
        alignItems="center"
        sx={{
          mb: 2,
        }}
      >
        <Box data-testid="cluster-definition" sx={{ display: 'grid' }}>
          <CodeBlock code={cluster.title} />
        </Box>
      </Grid>
      <Grid size={12}>
        <Button component={Link} variant="contained" to={newRuleURL}>
          create rule from cluster
        </Button>
      </Grid>
    </>
  );
};

const ClusterInfo = () => {
  const {
    project,
    algorithm: clusterAlgorithm,
    id: clusterId,
  } = useContext(ClusterContext);

  const {
    isLoading,
    isSuccess,
    data: cluster,
    error,
  } = useFetchCluster(project, clusterAlgorithm, clusterId);

  let criteriaName = '';
  if (clusterAlgorithm.startsWith('testname-')) {
    criteriaName = 'Test name cluster';
  } else if (clusterAlgorithm.startsWith('reason-')) {
    criteriaName = 'Failure reason cluster';
  }

  return (
    <Paper data-cy="cluster-info" elevation={3} sx={{ pt: 2, pb: 2, mt: 1 }}>
      <Container maxWidth={false}>
        <PanelHeading gutterBottom>{criteriaName}</PanelHeading>
        {isLoading && <CentralizedProgress />}
        {error && <LoadErrorAlert entityName="cluster" error={error} />}
        {isSuccess && cluster && <ClusterDetails cluster={cluster} />}
      </Container>
    </Paper>
  );
};

export default ClusterInfo;
