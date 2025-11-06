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

import { useQuery, UseQueryResult } from '@tanstack/react-query';

import { useMetricsService } from '@/clusters/services/services';
import { prpcRetrier } from '@/clusters/tools/prpc_retrier';
import {
  ListProjectMetricsRequest,
  ProjectMetric,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';

const useFetchMetrics = (
  project: string,
): UseQueryResult<readonly ProjectMetric[], Error> => {
  const metricsService = useMetricsService();
  return useQuery({
    queryKey: ['metrics', project],

    queryFn: async () => {
      const request: ListProjectMetricsRequest = {
        parent: 'projects/' + project,
      };

      const response = await metricsService.ListForProject(request);
      return response.metrics || [];
    },

    retry: prpcRetrier,
  });
};

export default useFetchMetrics;
