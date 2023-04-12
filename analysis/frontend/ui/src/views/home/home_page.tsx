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
  LinkProps,
  Link as RouterLink,
} from 'react-router-dom';

import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import { styled } from '@mui/material/styles';
import {
  Alert,
  AlertTitle,
  Link,
} from '@mui/material';

import CentralizedProgress from '@/components/centralized_progress/centralized_progress';
import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import PageHeading from '@/components/headings/page_heading/page_heading';

import useFetchProjects from '@/hooks/use_fetch_projects';
import { loginLink } from '@/tools/urlHandling/links';

const ProjectCard = styled(RouterLink)<LinkProps>(() => ({
  'padding-top': '2rem',
  'padding-bottom': '2rem',
  'display': 'flex',
  'justify-content': 'center',
  'align-items': 'center',
  'box-shadow': '0px 2px 1px -1px rgb(0 0 0 / 20%), 0px 1px 1px 0px rgb(0 0 0 / 14%), 0px 1px 3px 0px rgb(0 0 0 / 12%)',
  'font-size': '1.5rem',
  'text-decoration': 'none',
  'color': 'black',
  'border-radius': '4px',
  'transition': 'transform .2s',
  '&:hover': {
    'transform': 'scale(1.1)',
  },
}));

const HomePage = () => {
  const { error, isLoading, data: projects } = useFetchProjects();
  return (
    <Container maxWidth="xl">
      <PageHeading>
        Projects
      </PageHeading>
      {
        window.isAnonymous && (
          <Alert
            severity="info"
            sx={{ mb: 2 }}>
            <AlertTitle>Some projects may be hidden</AlertTitle>
            Please <Link href={loginLink('/')}>log in</Link> to view all projects you have access to.
          </Alert>
        )
      }
      {
        isLoading && (
          <CentralizedProgress />
        )
      }
      {
        error && (
          <Grid container item>
            <LoadErrorAlert
              entityName="projects"
              error={error} />
          </Grid>
        )
      }
      <Grid container spacing={2} id="project-cards">
        {
          projects && projects.map((project) => (
            <Grid
              key={project.name}
              item xs={2}>
              <ProjectCard
                to={`/p/${project.project}/clusters`}
              >
                {project.displayName}
              </ProjectCard>
            </Grid>
          ))
        }
      </Grid>
    </Container>
  );
};

export default HomePage;
