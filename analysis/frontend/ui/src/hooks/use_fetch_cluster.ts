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

import { useQuery, UseQueryResult } from 'react-query';

import {
  getClustersService,
} from '@/services/services';
import { prpcRetrier } from '@/legacy_services/shared_models';
import { Cluster, GetClusterRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

const useFetchCluster = (
    project: string,
    algorithm: string,
    id: string,
): UseQueryResult<Cluster, Error> => {
  const clustersService = getClustersService();
  return useQuery(['cluster', project, algorithm, id], async () => {
    const request: GetClusterRequest = {
      name: `projects/${encodeURIComponent(project)}/clusters/${encodeURIComponent(algorithm)}/${encodeURIComponent(id)}`,
    };

    const response = await clustersService.get(request);
    return response;
  }, {
    retry: prpcRetrier,
  });
};

export default useFetchCluster;
