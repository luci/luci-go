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
  ListMetricsRequest,
  getMetricsService,
  Metric,
} from '@/services/metrics';
import { prpcRetrier } from '@/services/shared_models';

const useFetchMetrics = (): UseQueryResult<Metric[], Error> => {
  const metricsService = getMetricsService();
  return useQuery(['metrics'], async () => {
    const request: ListMetricsRequest = {};

    const response = await metricsService.list(request);
    return response.metrics || [];
  }, {
    retry: prpcRetrier,
  });
};

export default useFetchMetrics;
