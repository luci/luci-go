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
import Container from '@mui/material/Container';
import HelpTooltip from '@/components/help_tooltip/help_tooltip';
import ClustersTable from '@/components/clusters_table/clusters_table';

const rulesDescription = 'Clusters are groups of related test failures. LUCI Analysis\'s clusters ' +
  'comprise clusters identified by algorithms (based on test name or failure reason) ' +
  'and clusters defined by a failure association rule (where the cluster contains all failures ' +
  'associated with a specific bug).';

const ClustersPage = () => {
  const { project } = useParams();
  return (
    <Container maxWidth={false}>
      <Grid container>
        <Grid item xs={8}>
          <h2>Clusters in project {project}<HelpTooltip text={rulesDescription}></HelpTooltip></h2>
        </Grid>
      </Grid>
      {(project) && (
        <ClustersTable project={project}></ClustersTable>
      )}
    </Container>
  );
};

export default ClustersPage;

