// Copyright 2023 The LUCI Authors.
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
  Card,
  CardActionArea,
  CardContent,
  Typography,
  styled,
} from '@mui/material';
import { Link } from 'react-router-dom';

import { ProjectListItem } from '@/common/services/milo_internal';

const ProjectCardContainer = styled(Card)({
  '&.AppProjectCard-large': {
    '.AppProjectCard-logo': {
      width: '134px',
      height: '134px',
    },
    '.AppProjectCard-logoText': {
      fontSize: '6rem',
      fontWeight: '300',
      width: '134px',
      lineHeight: '137px',
    },
    '.AppProjectCard-projectLabel': {
      fontSize: '1.25rem',
      fontWeight: '500',
      lineHeight: '50px',
    },
  },
  '&.AppProjectCard-small': {
    '.MuiCardContent-root': {
      paddingBottom: 0,
    },
    '.AppProjectCard-logo': {
      width: '84px',
      height: '84px',
    },
    '.AppProjectCard-logoText': {
      fontSize: '3.75rem',
      fontWeight: '300',
      width: '84px',
      lineHeight: '87px',
    },
  },
});

interface ProjectCardProps {
  readonly project: ProjectListItem;
  readonly onSelectProjectNotification: (projectId: string) => void;
  readonly variant?: 'large' | 'small';
}

export function ProjectCard({
  project,
  onSelectProjectNotification,
  variant = 'small',
}: ProjectCardProps) {
  return (
    <ProjectCardContainer
      className={`AppProjectCard-${variant}`}
      elevation={variant === 'small' ? undefined : 5}
      variant={variant === 'small' ? 'outlined' : 'elevation'}
    >
      <CardActionArea
        component={Link}
        onClick={() => onSelectProjectNotification(project.id)}
        to={`/ui/p/${project.id}/builders`}
      >
        <CardContent>
          {project.logoUrl ? (
            <img
              src={project.logoUrl}
              alt={project.id}
              className="AppProjectCard-logo"
              style={{
                objectFit: 'cover',
              }}
            />
          ) : (
            <Typography
              variant="h1"
              className="AppProjectCard-logoText"
              sx={{
                textAlign: 'center',
                objectFit: 'cover',
              }}
            >
              {project.id.slice(0, 2)}
            </Typography>
          )}
        </CardContent>
        <Typography
          className="AppProjectCard-projectLabel"
          sx={{
            textAlign: 'center',
            color: '#fff',
            backgroundColor: 'rgba(0,0,0,0.6)',
          }}
        >
          {project.id}
        </Typography>
      </CardActionArea>
    </ProjectCardContainer>
  );
}
