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

import { Card, CardActionArea, CardContent, Typography } from '@mui/material';
import { Link } from 'react-router-dom';

import { ProjectListItem } from '@/common/services/milo_internal';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';

interface ProjectListDisplayProps {
  readonly projects: ProjectListItem[];
  readonly isLoading: boolean;
  readonly variant?: 'large';
  readonly onSelectProjectNotification: (projectId: string) => void;
}

export function ProjectListDisplay({
  projects,
  isLoading,
  variant,
  onSelectProjectNotification,
}: ProjectListDisplayProps) {
  return (
    <>
      <div
        style={{
          display: 'flex',
          gap: '10px',
          flexWrap: 'wrap',
          justifyContent: 'center',
          margin: '0 auto',
        }}
      >
        {projects.map((project) =>
          variant == 'large' ? (
            <LargeProjectCard
              key={project.id}
              project={project}
              onSelectProjectNotification={onSelectProjectNotification}
            />
          ) : (
            <SmallProjectCard
              key={project.id}
              project={project}
              onSelectProjectNotification={onSelectProjectNotification}
            />
          ),
        )}
      </div>
      {isLoading && (
        <Typography component="span">
          Loading <DotSpinner />
        </Typography>
      )}
    </>
  );
}

type ToAttrWorkaround = any;

interface ProjectCardProps {
  readonly project: ProjectListItem;
  readonly onSelectProjectNotification: (projectId: string) => void;
}
const LargeProjectCard = ({
  project,
  onSelectProjectNotification,
}: ProjectCardProps) => {
  return (
    <Card key={project.id} elevation={5}>
      <CardActionArea
        LinkComponent={Link}
        onClick={() => onSelectProjectNotification(project.id)}
        {...({ to: `/ui/p/${project.id}/builders` } as ToAttrWorkaround)}
      >
        <CardContent>
          {project.logoUrl ? (
            <img
              src={project.logoUrl}
              alt={project.id}
              style={{
                objectFit: 'cover',
                width: '134px',
                height: '134px',
              }}
            />
          ) : (
            <Typography
              variant="h1"
              sx={{
                textAlign: 'center',
                objectFit: 'cover',
                width: '134px',
                lineHeight: '137px',
              }}
            >
              {project.id.slice(0, 2)}
            </Typography>
          )}
        </CardContent>
        <Typography
          variant="h6"
          sx={{
            lineHeight: '50px',
            textAlign: 'center',
            color: '#fff',
            backgroundColor: 'rgba(0,0,0,0.6)',
          }}
        >
          {project.id}
        </Typography>
      </CardActionArea>
    </Card>
  );
};

const SmallProjectCard = ({
  project,
  onSelectProjectNotification,
}: ProjectCardProps) => {
  return (
    <Card key={project.id} variant="outlined">
      <CardActionArea
        LinkComponent={Link}
        onClick={() => onSelectProjectNotification(project.id)}
        {...({ to: `/ui/p/${project.id}/builders` } as ToAttrWorkaround)}
      >
        <CardContent sx={{ paddingBottom: '0' }}>
          {project.logoUrl ? (
            <img
              src={project.logoUrl}
              alt={project.id}
              style={{
                objectFit: 'cover',
                width: '84px',
                height: '84px',
              }}
            />
          ) : (
            <Typography
              variant="h2"
              sx={{
                textAlign: 'center',
                objectFit: 'cover',
                width: '84px',
                lineHeight: '87px',
              }}
            >
              {project.id.slice(0, 2)}
            </Typography>
          )}
        </CardContent>
        <Typography
          sx={{
            textAlign: 'center',
            color: '#fff',
            backgroundColor: 'rgba(0,0,0,0.6)',
          }}
        >
          {project.id}
        </Typography>
      </CardActionArea>
    </Card>
  );
};
