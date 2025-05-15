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

import { useQuery } from '@tanstack/react-query';

import { useProjectsService } from '@/clusters/services/services';
import { prpcRetrier } from '@/clusters/tools/prpc_retrier';
import { ListProjectsRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';

const useFetchProjects = () => {
  const service = useProjectsService();
  const request: ListProjectsRequest = {};
  return useQuery({
    queryKey: ['projects'],

    queryFn: async () => {
      const response = await service.List(request);
      // Chromium milestone projects are explicitly ignored by the backend, match this in the frontend.
      return response.projects.filter(
        (p) => !/^(chromium|chrome)-m[0-9]+$/.test(p.project),
      );
    },

    retry: prpcRetrier,
  });
};

export default useFetchProjects;
