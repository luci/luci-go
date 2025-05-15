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

import useFetchCluster from './use_fetch_cluster';
import useFetchMetrics from './use_fetch_metrics';

const useFetchClusterAndMetrics = (
  project: string,
  algorithm: string,
  id: string,
) => {
  const {
    data: cluster,
    error: clusterError,
    isPending: clusterIsLoading,
  } = useFetchCluster(project, algorithm, id);
  const {
    data: metrics,
    error: metricsError,
    isPending: metricsIsLoading,
  } = useFetchMetrics(project);
  return {
    isLoading: metricsIsLoading || clusterIsLoading,
    metricsError: metricsError,
    clusterError: clusterError,
    cluster: cluster,
    metrics: metrics,
  };
};

export default useFetchClusterAndMetrics;
