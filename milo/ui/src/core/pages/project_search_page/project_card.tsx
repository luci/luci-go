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
  CardMedia,
  Typography,
  styled,
} from '@mui/material';
import { Link as RouterLink } from 'react-router';

import { getProjectURLPath } from '@/common/tools/url_utils';
import { ProjectListItem } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

const ProjectCardContainer = styled(Card)({
  '& .MuiCardActionArea-root': {
    width: '100%',
    height: '100%',
    // Anchor the child's absolute position.
    position: 'relative',
  },
  '& .AppProjectCard-logoContainer': {
    // Fill the whole container with padding to get a square container.
    width: '100%',
    padding: '50%',
    boxSizing: 'border-box',

    // Anchor the child's absolute position.
    position: 'relative',
    '& > *': {
      // Ensure the child don't interfere with the parent's size.
      position: 'absolute',

      // Vertically and horizontally center the content.
      margin: '0',
      top: '50%',
      left: '50%',
      transform: 'translate(-50%, -50%)',

      // Make the content fill the parent.
      maxWidth: '100%',
      maxHeight: '100%',
      objectFit: 'contain',
    },
  },
  '& .AppProjectCard-projectLabelContainer': {
    // Position the container at the bottom with set width but unrestricted
    // height. This allows the project label to grow upwards without overflowing
    // or changing parent's size. This is useful for projects with long labels.
    position: 'absolute',
    bottom: '0%',
    left: '0%',
    width: '100%',
  },
  '.AppProjectCard-projectLabel': {
    padding: '3px 1px',
    textAlign: 'center',
    color: '#fff',
    backgroundColor: 'rgba(0,0,0,0.6)',
  },

  '&.AppProjectCard-large': {
    height: '206px',
    width: '170px',
    '.MuiCardMedia-root': {
      fontSize: '6rem',
      fontWeight: '300',
    },
    '.AppProjectCard-projectLabel': {
      fontSize: '1.25rem',
      fontWeight: '500',
    },
  },

  '&.AppProjectCard-small': {
    height: '160px',
    width: '130px',
    '.MuiCardMedia-root': {
      fontSize: '4rem',
      fontWeight: '300',
    },
    '.AppProjectCard-projectLabel': {
      fontSize: '1rem',
    },
  },
});

export interface ProjectCardProps {
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
      elevation={variant === 'small' ? undefined : 5}
      variant={variant === 'small' ? 'outlined' : 'elevation'}
      className={`AppProjectCard-${variant}`}
    >
      <CardActionArea
        onClick={() => onSelectProjectNotification(project.id)}
        component={RouterLink}
        to={getProjectURLPath(project.id)}
      >
        <div className="AppProjectCard-logoContainer">
          {project.logoUrl ? (
            <CardMedia
              component="img"
              image={project.logoUrl}
              alt={project.id}
              className="AppProjectCard-logoImage"
            />
          ) : (
            <CardMedia component="h1" className="AppProjectCard-logoText">
              {project.id.slice(0, 2)}
            </CardMedia>
          )}
        </div>
        <CardContent
          sx={{ p: 0 }}
          className="AppProjectCard-projectLabelContainer"
        >
          <Typography component="h5" className="AppProjectCard-projectLabel">
            {project.id}
          </Typography>
        </CardContent>
      </CardActionArea>
    </ProjectCardContainer>
  );
}
