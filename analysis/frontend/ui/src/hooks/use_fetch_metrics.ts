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

import { getMetricsService } from '@/services/services';
import { ListProjectMetricsRequest, ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';
import { prpcRetrier } from '@/tools/prpc_retrier';

const useFetchMetrics = (project: string, onSuccess?: (data: ProjectMetric[]) => void): UseQueryResult<ProjectMetric[], Error> => {
  const metricsService = getMetricsService();
  return useQuery(['metrics', project], async () => {
    const request: ListProjectMetricsRequest = {
      parent: 'projects/' + project,
    };

    const response = await metricsService.listForProject(request);
    return response.metrics || [];
  }, {
    retry: prpcRetrier,
    onSuccess: onSuccess,
  });
};

export default useFetchMetrics;
