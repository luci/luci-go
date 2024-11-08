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

import Button from '@mui/material/Button';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import { Link, useParams } from 'react-router-dom';

import PageHeading from '@/clusters/components/headings/page_heading/page_heading';
import HelpTooltip from '@/clusters/components/help_tooltip/help_tooltip';
import RulesTable from '@/clusters/components/rules_table/rules_table';

const rulesDescription =
  'Rules define an association between failures and bugs. LUCI Analysis uses these ' +
  'associations to calculate bug impact, automatically adjust bug priority and verified status, and ' +
  'to surface bugs for failures in the MILO test results UI.';

const RulesPage = () => {
  const { project } = useParams();
  return (
    <Container maxWidth={false}>
      <Grid container>
        <Grid item xs={8}>
          <PageHeading>
            Rules in project {project}
            <HelpTooltip text={rulesDescription}></HelpTooltip>
          </PageHeading>
        </Grid>
        <Grid item xs={4} sx={{ textAlign: 'right' }}>
          <Button
            component={Link}
            variant="contained"
            to="new"
            sx={{ marginBlockStart: '20px' }}
          >
            New Rule
          </Button>
        </Grid>
      </Grid>
      {project && <RulesTable project={project}></RulesTable>}
    </Container>
  );
};

export default RulesPage;
