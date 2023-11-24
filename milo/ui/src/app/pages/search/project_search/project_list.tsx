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

import { Box, CircularProgress, Typography } from '@mui/material';
import { useEffect, useMemo } from 'react';
import { useLocalStorage } from 'react-use';

import { useInfinitePrpcQuery } from '@/common/hooks/legacy_prpc_query/use_infinite_prpc_query';
import { MiloInternal } from '@/common/services/milo_internal';

import { ProjectListDisplay } from './project_list_display';

interface ProjectListProps {
  readonly searchQuery: string;
}

const RECENTLY_SELECTED_PROJECTS_KEY = 'recentlySelectedProjects';

export function ProjectList({ searchQuery }: ProjectListProps) {
  const { data, isError, error, isLoading, fetchNextPage, hasNextPage } =
    useInfinitePrpcQuery({
      host: '',
      insecure: location.protocol === 'http:',
      Service: MiloInternal,
      method: 'listProjects',
      request: {
        pageSize: 10000,
      },
    });

  if (isError) {
    throw error;
  }

  // Keep loading projects until all pages are loaded.
  useEffect(() => {
    if (!isLoading && hasNextPage) {
      fetchNextPage();
    }
  }, [fetchNextPage, isLoading, hasNextPage]);

  // Computes `projects` separately so it's not re-computed when only
  // `searchQuery` is updated.
  const projects = useMemo(
    () =>
      data?.pages
        .flatMap((p) => p.projects || [])
        .map((p) => {
          return [
            p,
            // Pre-compute to support case-insensitive searching.
            p.id.toLowerCase(),
          ] as const;
        }) || [],
    [data],
  );

  const [recentProjectIds = [], setRecentProjectIds] = useLocalStorage<
    readonly string[]
  >(RECENTLY_SELECTED_PROJECTS_KEY);
  const saveRecentProject = (projectId: string) => {
    const newProjects = [
      projectId,
      ...recentProjectIds.filter((id) => id !== projectId).slice(0, 2),
    ];
    setRecentProjectIds(newProjects);
  };

  // Get other project properties (e.g. `logoUrl`) once them become available.
  const recentProjects = useMemo(
    () =>
      recentProjectIds.map(
        (id) =>
          projects.find(([project, _]) => project.id === id)?.[0] || { id },
      ),
    [projects, recentProjectIds],
  );

  // Filter projects.
  const filteredProjects = useMemo(() => {
    const parts = searchQuery.toLowerCase().split(' ');
    return projects
      .filter(([_, lowerCaseProject]) =>
        parts.every((part) => lowerCaseProject.includes(part)),
      )
      .map(([project, _]) => project);
  }, [projects, searchQuery]);

  return (
    <>
      {searchQuery === '' && recentProjects.length > 0 && (
        <>
          <Box sx={{ textAlign: 'center' }}>
            <Typography>Recent Projects</Typography>
          </Box>
          <ProjectListDisplay
            projects={recentProjects}
            onSelectProjectNotification={saveRecentProject}
            variant="large"
          />
          <Box sx={{ textAlign: 'center', mt: 5 }}>
            <Typography>All Projects</Typography>
          </Box>
        </>
      )}
      <ProjectListDisplay
        projects={filteredProjects}
        onSelectProjectNotification={saveRecentProject}
      />
      <Box sx={{ textAlign: 'center', padding: '20px' }}>
        {!isLoading && filteredProjects.length === 0 && (
          <Typography>No projects match your filter.</Typography>
        )}
        {isLoading && <CircularProgress />}
      </Box>
    </>
  );
}
