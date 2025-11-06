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

import { useQuery, UseQueryResult } from '@tanstack/react-query';

import { useClustersService } from '@/clusters/services/services';
import { prpcRetrier } from '@/clusters/tools/prpc_retrier';
import {
  DistinctClusterFailure,
  QueryClusterFailuresRequest,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';

const useFetchClusterFailures = (
  project: string,
  algorithm: string,
  id: string,
  filterToMetric: ProjectMetric | undefined,
): UseQueryResult<readonly DistinctClusterFailure[], Error> => {
  const service = useClustersService();

  return useQuery({
    queryKey: [
      'clusterFailures',
      project,
      algorithm,
      id,
      filterToMetric?.metricId,
    ],

    queryFn: async () => {
      const request: QueryClusterFailuresRequest = {
        parent: `projects/${project}/clusters/${algorithm}/${id}/failures`,
        metricFilter: filterToMetric?.name || '',
      };
      const response = await service.QueryClusterFailures(request);
      return response.failures || [];
    },

    retry: prpcRetrier,
  });
};

export default useFetchClusterFailures;
