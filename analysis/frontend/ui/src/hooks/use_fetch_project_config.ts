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
  useQuery,
  UseQueryResult,
} from 'react-query';

import {
  GetProjectConfigRequest,
  getProjectsService,
  ProjectConfig,
} from '@/legacy_services/project';
import { prpcRetrier } from '@/legacy_services/shared_models';

export const useFetchProjectConfig = (
    project: string,
): UseQueryResult<ProjectConfig, Error> => {
  const projectsService = getProjectsService();
  return useQuery(['projectconfig', project], async () => {
    if (!project) {
      throw new Error('invariant violated: project should be set');
    }
    const request: GetProjectConfigRequest = {
      name: `projects/${encodeURIComponent(project)}/config`,
    };
    return await projectsService.getConfig(request);
  }, {
    enabled: !!project,
    retry: prpcRetrier,
  });
};
