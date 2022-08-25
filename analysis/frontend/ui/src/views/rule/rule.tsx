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

import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';

import ImpactSection from '../../components/impact_section/impact_section';
import RecentFailuresSection from '../../components/recent_failures_section/recent_failures_section';
import RuleTopPanel from '../../components/rule/rule_top_panel/rule_top_panel';

const Rule = () => {
  const { project, id } = useParams();

  return (
    <Container className='mt-1' maxWidth={false}>
      <Grid sx={{ mt: 1 }} container spacing={2}>
        <Grid item xs={12}>
          {(project && id) && (
            <RuleTopPanel project={project} ruleId={id} />
          )}
        </Grid>
        <Grid item xs={12}>
          <ImpactSection />
        </Grid>
        <Grid item xs={12}>
          <RecentFailuresSection />
        </Grid>
      </Grid>
    </Container>
  );
};

export default Rule;
